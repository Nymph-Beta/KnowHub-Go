package pipeline

import (
	"context"
	"fmt"
	"strings"

	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/internal/repository"
	"pai_smart_go_v2/pkg/log"
	"pai_smart_go_v2/pkg/tasks"
	"pai_smart_go_v2/pkg/tika"

	"github.com/minio/minio-go/v7"
)

const (
	defaultTextChunkSize    = 1000
	defaultTextChunkOverlap = 100
)

type Processor struct {
	tikaClient    *tika.Client
	minioClient   *minio.Client
	bucketName    string
	docVectorRepo repository.DocumentVectorRepository
}

func NewProcessor(
	tikaClient *tika.Client,
	minioClient *minio.Client,
	bucketName string,
	docVectorRepo repository.DocumentVectorRepository,
) *Processor {
	return &Processor{
		tikaClient:    tikaClient,
		minioClient:   minioClient,
		bucketName:    bucketName,
		docVectorRepo: docVectorRepo,
	}
}

func (p *Processor) Process(ctx context.Context, task tasks.FileProcessingTask) error {
	if p.tikaClient == nil {
		return fmt.Errorf("tika client is nil")
	}
	if p.minioClient == nil {
		return fmt.Errorf("minio client is nil")
	}
	if p.docVectorRepo == nil {
		return fmt.Errorf("document vector repository is nil")
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

	textLength := len([]rune(text))
	log.Infof("[Processor] Tika 提取文本成功: md5=%s, textLength=%d", task.FileMD5, textLength)

	if strings.TrimSpace(text) == "" {
		log.Warnf("[Processor] 文本为空，跳过分块: md5=%s", task.FileMD5)
		return nil
	}

	chunks, err := splitText(text, defaultTextChunkSize, defaultTextChunkOverlap)
	if err != nil {
		return fmt.Errorf("split text failed: %w", err)
	}
	if len(chunks) == 0 {
		log.Warnf("[Processor] 分块结果为空，跳过写库: md5=%s", task.FileMD5)
		return nil
	}

	if err := p.docVectorRepo.DeleteByFileMD5(task.FileMD5); err != nil {
		log.Warnf("[Processor] 删除旧分块失败，继续写入: md5=%s err=%v", task.FileMD5, err)
	}

	vectors := buildDocumentVectors(task, chunks)
	if err := p.docVectorRepo.BatchCreate(vectors); err != nil {
		return fmt.Errorf("batch create document vectors failed: %w", err)
	}

	log.Infof("[Processor] 文本分块完成: md5=%s, chunks=%d", task.FileMD5, len(vectors))
	log.Infof("[Processor] 批量写入 document_vectors 成功: md5=%s", task.FileMD5)
	return nil
}

func splitText(text string, chunkSize int, overlap int) ([]string, error) {
	if chunkSize <= 0 {
		return nil, fmt.Errorf("chunk_size must be greater than 0")
	}
	if overlap < 0 {
		return nil, fmt.Errorf("overlap must be greater than or equal to 0")
	}
	if chunkSize <= overlap {
		return nil, fmt.Errorf("chunk_size must be greater than overlap")
	}
	if text == "" {
		return []string{}, nil
	}

	runes := []rune(text)
	step := chunkSize - overlap
	chunks := make([]string, 0, (len(runes)+step-1)/step)

	for start := 0; start < len(runes); start += step {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}

		chunk := string(runes[start:end])
		if strings.TrimSpace(chunk) != "" {
			chunks = append(chunks, chunk)
		}
		if end == len(runes) {
			break
		}
	}

	return chunks, nil
}

func buildDocumentVectors(task tasks.FileProcessingTask, chunks []string) []model.DocumentVector {
	vectors := make([]model.DocumentVector, 0, len(chunks))
	for i, chunk := range chunks {
		vectors = append(vectors, model.DocumentVector{
			FileMD5:      task.FileMD5,
			ChunkID:      i,
			TextContent:  chunk,
			ModelVersion: "",
			UserID:       task.UserID,
			OrgTag:       task.OrgTag,
			IsPublic:     task.IsPublic,
		})
	}
	return vectors
}
