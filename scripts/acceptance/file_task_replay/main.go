package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"pai_smart_go_v2/pkg/tasks"

	kafkago "github.com/segmentio/kafka-go"
)

type replayResult struct {
	FileMD5   string `json:"fileMd5"`
	ObjectKey string `json:"objectKey"`
	Count     int    `json:"count"`
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func main() {
	brokersFlag := flag.String("brokers", "127.0.0.1:9093", "Comma-separated Kafka brokers")
	topic := flag.String("topic", "file-processing", "Kafka topic")
	fileMD5 := flag.String("file-md5", "", "File MD5")
	fileName := flag.String("file-name", "", "File name")
	userID := flag.Uint("user-id", 0, "User ID")
	orgTag := flag.String("org-tag", "", "Org tag")
	isPublic := flag.Bool("is-public", false, "Is public")
	objectKey := flag.String("object-key", "", "Object key")
	count := flag.Int("count", 1, "Replay count")
	flag.Parse()

	if strings.TrimSpace(*fileMD5) == "" || strings.TrimSpace(*fileName) == "" || strings.TrimSpace(*objectKey) == "" || *userID == 0 {
		fmt.Fprintln(os.Stderr, "file-md5, file-name, user-id and object-key are required")
		os.Exit(1)
	}
	if *count <= 0 {
		fmt.Fprintln(os.Stderr, "count must be greater than 0")
		os.Exit(1)
	}

	brokers := make([]string, 0)
	for _, broker := range strings.Split(*brokersFlag, ",") {
		broker = strings.TrimSpace(broker)
		if broker != "" {
			brokers = append(brokers, broker)
		}
	}
	if len(brokers) == 0 {
		fmt.Fprintln(os.Stderr, "at least one broker is required")
		os.Exit(1)
	}

	writer := &kafkago.Writer{
		Addr:         kafkago.TCP(brokers...),
		Topic:        *topic,
		Balancer:     &kafkago.LeastBytes{},
		RequiredAcks: kafkago.RequireAll,
		BatchTimeout: 20 * time.Millisecond,
		Async:        false,
	}
	defer writer.Close()

	task := tasks.FileProcessingTask{
		FileMD5:   *fileMD5,
		FileName:  *fileName,
		UserID:    uint(*userID),
		OrgTag:    *orgTag,
		IsPublic:  *isPublic,
		ObjectKey: *objectKey,
	}
	payload, err := json.Marshal(task)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal task failed: %v\n", err)
		os.Exit(1)
	}

	messages := make([]kafkago.Message, 0, *count)
	for i := 0; i < *count; i++ {
		messages = append(messages, kafkago.Message{
			Key:   []byte(task.FileMD5),
			Value: payload,
			Time:  time.Now(),
		})
	}

	if err := writer.WriteMessages(context.Background(), messages...); err != nil {
		fmt.Fprintf(os.Stderr, "write kafka message failed: %v\n", err)
		os.Exit(1)
	}

	printJSON(replayResult{
		FileMD5:   task.FileMD5,
		ObjectKey: task.ObjectKey,
		Count:     *count,
	})
}
