package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"pai_smart_go_v2/internal/config"
	"pai_smart_go_v2/pkg/log"
	"pai_smart_go_v2/pkg/tasks"

	"github.com/go-redis/redis/v8"
	kafkago "github.com/segmentio/kafka-go"
)

const (
	defaultMaxRetry           = 3
	defaultRetryKeyTTLSeconds = 24 * 60 * 60
)

type producerWriter interface {
	WriteMessages(ctx context.Context, msgs ...kafkago.Message) error
	Close() error
}

type consumerReader interface {
	FetchMessage(ctx context.Context) (kafkago.Message, error)
	CommitMessages(ctx context.Context, msgs ...kafkago.Message) error
	Close() error
}

// TaskProcessor 由下游处理器实现，Kafka consumer 仅负责调度消息。
type TaskProcessor interface {
	Process(ctx context.Context, task tasks.FileProcessingTask) error
}

// RetryStore 抽象 consumer 失败计数存储，实现可用 Redis 或测试替身。
type RetryStore interface {
	Incr(ctx context.Context, key string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	Del(ctx context.Context, key string) error
}

// FileTaskProducer 给业务层注入发送能力，避免直接依赖全局函数。
type FileTaskProducer interface {
	ProduceFileTask(ctx context.Context, task tasks.FileProcessingTask) error
}

// ProducerClient 是 FileTaskProducer 的默认实现。
type ProducerClient struct{}

func NewProducerClient() *ProducerClient {
	return &ProducerClient{}
}

func (p *ProducerClient) ProduceFileTask(ctx context.Context, task tasks.FileProcessingTask) error {
	return ProduceFileTask(ctx, task)
}

type redisRetryStore struct {
	client *redis.Client
}

func NewRedisRetryStore(client *redis.Client) RetryStore {
	if client == nil {
		return nil
	}
	return &redisRetryStore{client: client}
}

func (s *redisRetryStore) Incr(ctx context.Context, key string) (int64, error) {
	return s.client.Incr(ctx, key).Result()
}

func (s *redisRetryStore) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return s.client.Expire(ctx, key, ttl).Err()
}

func (s *redisRetryStore) Del(ctx context.Context, key string) error {
	return s.client.Del(ctx, key).Err()
}

var (
	producer  producerWriter
	newReader = func(cfg kafkago.ReaderConfig) consumerReader {
		return kafkago.NewReader(cfg)
	}
)

func InitProducer(cfg config.KafkaConfig) error {
	if len(cfg.Brokers) == 0 {
		return fmt.Errorf("kafka brokers is empty")
	}
	if strings.TrimSpace(cfg.Topic) == "" {
		return fmt.Errorf("kafka topic is empty")
	}

	producer = &kafkago.Writer{
		Addr:         kafkago.TCP(cfg.Brokers...),
		Topic:        cfg.Topic,
		Balancer:     &kafkago.LeastBytes{},
		RequiredAcks: kafkago.RequireAll,
		BatchTimeout: 20 * time.Millisecond,
		Async:        false,
	}
	return nil
}

func CloseProducer() error {
	if producer == nil {
		return nil
	}
	return producer.Close()
}

func ProduceFileTask(ctx context.Context, task tasks.FileProcessingTask) error {
	if producer == nil {
		return fmt.Errorf("kafka producer is not initialized")
	}
	if strings.TrimSpace(task.FileMD5) == "" {
		return fmt.Errorf("file_md5 is required")
	}
	if strings.TrimSpace(task.ObjectKey) == "" {
		return fmt.Errorf("object_key is required")
	}

	payload, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal task failed: %w", err)
	}

	msg := kafkago.Message{
		Key:   []byte(task.FileMD5),
		Value: payload,
		Time:  time.Now(),
	}
	if err := producer.WriteMessages(ctx, msg); err != nil {
		return fmt.Errorf("write kafka message failed: %w", err)
	}

	log.Infof("[Kafka] 文件处理任务发送成功: MD5=%s", task.FileMD5)
	return nil
}

func StartConsumer(
	ctx context.Context,
	cfg config.KafkaConfig,
	retryStore RetryStore,
	processor TaskProcessor,
) error {
	if len(cfg.Brokers) == 0 {
		return fmt.Errorf("kafka brokers is empty")
	}
	if strings.TrimSpace(cfg.Topic) == "" {
		return fmt.Errorf("kafka topic is empty")
	}
	if strings.TrimSpace(cfg.GroupID) == "" {
		return fmt.Errorf("kafka group_id is empty")
	}
	if processor == nil {
		return fmt.Errorf("task processor is nil")
	}
	if retryStore == nil {
		return fmt.Errorf("retry store is nil")
	}

	reader := newReader(kafkago.ReaderConfig{
		Brokers:        cfg.Brokers,
		Topic:          cfg.Topic,
		GroupID:        cfg.GroupID,
		CommitInterval: 0, // 手动提交 offset
		MinBytes:       1,
		MaxBytes:       10e6,
		MaxWait:        2 * time.Second,
	})
	defer func() {
		_ = reader.Close()
	}()

	maxRetry := cfg.MaxRetry
	if maxRetry <= 0 {
		maxRetry = defaultMaxRetry
	}
	retryTTL := time.Duration(cfg.RetryKeyTTLSeconds) * time.Second
	if cfg.RetryKeyTTLSeconds <= 0 {
		retryTTL = defaultRetryKeyTTLSeconds * time.Second
	}

	for {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			log.Errorf("[Consumer] 拉取消息失败: %v", err)
			continue
		}

		if err := consumeOne(ctx, reader, msg, retryStore, processor, maxRetry, retryTTL); err != nil {
			log.Errorf("[Consumer] 消息处理流程异常: %v", err)
		}
	}
}

func consumeOne(
	ctx context.Context,
	reader consumerReader,
	msg kafkago.Message,
	retryStore RetryStore,
	processor TaskProcessor,
	maxRetry int,
	retryTTL time.Duration,
) error {
	log.Infof("[Consumer] 收到 Kafka 消息: topic=%s, partition=%d, offset=%d", msg.Topic, msg.Partition, msg.Offset)

	var task tasks.FileProcessingTask
	if err := json.Unmarshal(msg.Value, &task); err != nil {
		log.Errorf("[Consumer] 消息反序列化失败，跳过并提交 offset: %v", err)
		return commitMessage(ctx, reader, msg)
	}

	if err := processor.Process(ctx, task); err != nil {
		attempts, retryErr := incrementRetryCount(ctx, retryStore, task.FileMD5, retryTTL)
		if retryErr != nil {
			return fmt.Errorf("increment retry count failed: %w", retryErr)
		}

		if attempts >= int64(maxRetry) {
			log.Errorf("[Consumer] 处理失败且超过重试上限，跳过: md5=%s attempts=%d err=%v", task.FileMD5, attempts, err)
			return commitMessage(ctx, reader, msg)
		}

		log.Errorf("[Consumer] 处理失败，等待 Kafka 重投递: md5=%s attempts=%d err=%v", task.FileMD5, attempts, err)
		return nil
	}

	if err := clearRetryCount(ctx, retryStore, task.FileMD5); err != nil {
		log.Errorf("[Consumer] 清理重试计数失败: %v", err)
	}

	if err := commitMessage(ctx, reader, msg); err != nil {
		return err
	}
	log.Infof("[Consumer] 文件任务处理成功: MD5=%s", task.FileMD5)
	return nil
}

func commitMessage(ctx context.Context, reader consumerReader, msg kafkago.Message) error {
	if err := reader.CommitMessages(ctx, msg); err != nil {
		return fmt.Errorf("commit message failed: %w", err)
	}
	return nil
}

func incrementRetryCount(ctx context.Context, retryStore RetryStore, fileMD5 string, ttl time.Duration) (int64, error) {
	key := retryKey(fileMD5)
	attempts, err := retryStore.Incr(ctx, key)
	if err != nil {
		return 0, err
	}
	if ttl > 0 {
		if expErr := retryStore.Expire(ctx, key, ttl); expErr != nil {
			return 0, expErr
		}
	}
	return attempts, nil
}

func clearRetryCount(ctx context.Context, retryStore RetryStore, fileMD5 string) error {
	key := retryKey(fileMD5)
	return retryStore.Del(ctx, key)
}

func retryKey(fileMD5 string) string {
	md5 := strings.TrimSpace(fileMD5)
	if md5 == "" {
		md5 = "unknown"
	}
	return fmt.Sprintf("kafka:retry:%s", md5)
}
