package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"pai_smart_go_v2/pkg/log"
	"testing"
	"time"

	"pai_smart_go_v2/pkg/tasks"

	kafkago "github.com/segmentio/kafka-go"
)

func init() {
	log.Init("debug", "json", "")
}

type fakeWriter struct {
	writeErr error
	msgs     []kafkago.Message
}

func (f *fakeWriter) WriteMessages(ctx context.Context, msgs ...kafkago.Message) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.msgs = append(f.msgs, msgs...)
	return nil
}

func (f *fakeWriter) Close() error { return nil }

type fakeReader struct {
	commitErr   error
	commitCount int
}

func (f *fakeReader) FetchMessage(ctx context.Context) (kafkago.Message, error) {
	return kafkago.Message{}, context.Canceled
}

func (f *fakeReader) CommitMessages(ctx context.Context, msgs ...kafkago.Message) error {
	if f.commitErr != nil {
		return f.commitErr
	}
	f.commitCount += len(msgs)
	return nil
}

func (f *fakeReader) Close() error { return nil }

type fakeRetryStore struct {
	incrErr   error
	expireErr error
	delErr    error
	counts    map[string]int64
}

func (f *fakeRetryStore) Incr(ctx context.Context, key string) (int64, error) {
	if f.incrErr != nil {
		return 0, f.incrErr
	}
	if f.counts == nil {
		f.counts = map[string]int64{}
	}
	f.counts[key]++
	return f.counts[key], nil
}

func (f *fakeRetryStore) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return f.expireErr
}

func (f *fakeRetryStore) Del(ctx context.Context, key string) error {
	if f.delErr != nil {
		return f.delErr
	}
	delete(f.counts, key)
	return nil
}

type fakeProcessor struct {
	processErr error
	called     int
}

func (f *fakeProcessor) Process(ctx context.Context, task tasks.FileProcessingTask) error {
	f.called++
	return f.processErr
}

func TestProduceFileTask_Success(t *testing.T) {
	oldProducer := producer
	defer func() { producer = oldProducer }()

	w := &fakeWriter{}
	producer = w

	task := tasks.FileProcessingTask{
		FileMD5:   "md5-a",
		FileName:  "a.pdf",
		UserID:    1,
		ObjectKey: "uploads/1/md5-a/a.pdf",
	}
	if err := ProduceFileTask(context.Background(), task); err != nil {
		t.Fatalf("ProduceFileTask() error = %v", err)
	}
	if len(w.msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(w.msgs))
	}

	var got tasks.FileProcessingTask
	if err := json.Unmarshal(w.msgs[0].Value, &got); err != nil {
		t.Fatalf("unmarshal produced message failed: %v", err)
	}
	if got.FileMD5 != "md5-a" || got.ObjectKey != "uploads/1/md5-a/a.pdf" {
		t.Fatalf("unexpected message payload: %+v", got)
	}
}

func TestConsumeOne_ProcessFailedBelowThreshold_NoCommit(t *testing.T) {
	reader := &fakeReader{}
	store := &fakeRetryStore{counts: map[string]int64{}}
	processor := &fakeProcessor{processErr: errors.New("tika error")}

	task := tasks.FileProcessingTask{
		FileMD5:   "md5-b",
		FileName:  "b.pdf",
		UserID:    2,
		ObjectKey: "uploads/2/md5-b/b.pdf",
	}
	raw, _ := json.Marshal(task)
	msg := kafkago.Message{Value: raw}

	err := consumeOne(context.Background(), reader, msg, store, processor, 3, time.Hour)
	if err != nil {
		t.Fatalf("consumeOne() error = %v", err)
	}
	if reader.commitCount != 0 {
		t.Fatalf("expected no commit, got %d", reader.commitCount)
	}
	if processor.called != 1 {
		t.Fatalf("expected processor called once, got %d", processor.called)
	}
}

func TestConsumeOne_ProcessFailedReachThreshold_Commit(t *testing.T) {
	reader := &fakeReader{}
	store := &fakeRetryStore{counts: map[string]int64{}}
	processor := &fakeProcessor{processErr: errors.New("tika error")}

	task := tasks.FileProcessingTask{
		FileMD5:   "md5-c",
		FileName:  "c.pdf",
		UserID:    3,
		ObjectKey: "uploads/3/md5-c/c.pdf",
	}
	raw, _ := json.Marshal(task)
	msg := kafkago.Message{Value: raw}

	store.counts[retryKey("md5-c")] = 2
	err := consumeOne(context.Background(), reader, msg, store, processor, 3, time.Hour)
	if err != nil {
		t.Fatalf("consumeOne() error = %v", err)
	}
	if reader.commitCount != 1 {
		t.Fatalf("expected commit once, got %d", reader.commitCount)
	}
}

func TestConsumeOne_Success_ShouldCommitAndClearRetry(t *testing.T) {
	reader := &fakeReader{}
	store := &fakeRetryStore{counts: map[string]int64{retryKey("md5-d"): 2}}
	processor := &fakeProcessor{}

	task := tasks.FileProcessingTask{
		FileMD5:   "md5-d",
		FileName:  "d.pdf",
		UserID:    4,
		ObjectKey: "uploads/4/md5-d/d.pdf",
	}
	raw, _ := json.Marshal(task)
	msg := kafkago.Message{Value: raw}

	err := consumeOne(context.Background(), reader, msg, store, processor, 3, time.Hour)
	if err != nil {
		t.Fatalf("consumeOne() error = %v", err)
	}
	if reader.commitCount != 1 {
		t.Fatalf("expected commit once, got %d", reader.commitCount)
	}
	if _, ok := store.counts[retryKey("md5-d")]; ok {
		t.Fatalf("expected retry key cleared")
	}
}
