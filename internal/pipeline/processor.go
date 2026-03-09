package pipeline

import (
	"context"
	"fmt"
	"strings"

	"pai_smart_go_v2/pkg/log"
	"pai_smart_go_v2/pkg/tasks"
	"pai_smart_go_v2/pkg/tika"

	"github.com/minio/minio-go/v7"
)

type Processor struct {
	tikaClient  *tika.Client
	minioClient *minio.Client
	bucketName  string
}

func NewProcessor(tikaClient *tika.Client, minioClient *minio.Client, bucketName string) *Processor {
	return &Processor{
		tikaClient:  tikaClient,
		minioClient: minioClient,
		bucketName:  bucketName,
	}
}

func (p *Processor) Process(ctx context.Context, task tasks.FileProcessingTask) error {
	if p.tikaClient == nil {
		return fmt.Errorf("tika client is nil")
	}
	if p.minioClient == nil {
		return fmt.Errorf("minio client is nil")
	}
	if strings.TrimSpace(task.ObjectKey) == "" {
		return fmt.Errorf("object key is empty")
	}

	log.Infof("[Processor] 开始处理文件: md5=%s objectKey=%s", task.FileMD5, task.ObjectKey)

	object, err := p.minioClient.GetObject(ctx, p.bucketName, task.ObjectKey, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("get object from minio failed: %w", err)
	}
	defer object.Close()

	if _, err := object.Stat(); err != nil {
		return fmt.Errorf("stat object failed: %w", err)
	}

	text, err := p.tikaClient.ExtractText(ctx, object, task.FileName)
	if err != nil {
		return fmt.Errorf("extract text by tika failed: %w", err)
	}

	log.Infof("[Processor] Tika 提取文本成功: md5=%s, textLength=%d", task.FileMD5, len([]rune(text)))
	return nil
}
