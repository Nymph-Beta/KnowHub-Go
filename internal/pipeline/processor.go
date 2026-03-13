package pipeline

import (
	"context"
	"fmt"
	"strings"

	"pai_smart_go_v2/internal/config"
	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/internal/repository"
	"pai_smart_go_v2/pkg/embedding"
	"pai_smart_go_v2/pkg/es"
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
	embedding     embedding.Client
	esClient      es.Client
	embeddingCfg  config.EmbeddingConfig
}

func NewProcessor(
	tikaClient *tika.Client,
	minioClient *minio.Client,
	bucketName string,
	docVectorRepo repository.DocumentVectorRepository,
	embeddingClient embedding.Client,
	esClient es.Client,
	embeddingCfg config.EmbeddingConfig,
) *Processor {
	return &Processor{
		tikaClient:    tikaClient,
		minioClient:   minioClient,
		bucketName:    bucketName,
		docVectorRepo: docVectorRepo,
		embedding:     embeddingClient,
		esClient:      esClient,
		embeddingCfg:  embeddingCfg,
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
	if p.embedding == nil {
		return fmt.Errorf("embedding client is nil")
	}
	if p.esClient == nil {
		return fmt.Errorf("elasticsearch client is nil")
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

	vectors := buildDocumentVectors(task, chunks, p.embeddingCfg.Model)
	if err := p.docVectorRepo.BatchCreate(vectors); err != nil {
		return fmt.Errorf("batch create document vectors failed: %w", err)
	}

	log.Infof("[Processor] 文本分块完成: md5=%s, chunks=%d", task.FileMD5, len(vectors))
	log.Infof("[Processor] 批量写入 document_vectors 成功: md5=%s", task.FileMD5)

	persistedVectors, err := p.docVectorRepo.FindByFileMD5(task.FileMD5)
	if err != nil {
		return fmt.Errorf("find document vectors by file_md5 failed: %w", err)
	}
	if len(persistedVectors) == 0 {
		log.Warnf("[Processor] 未找到已写入的 chunk，跳过向量化: md5=%s", task.FileMD5)
		return nil
	}

	esDocs, dims, err := p.vectorizeDocuments(ctx, persistedVectors)
	if err != nil {
		return fmt.Errorf("vectorize document chunks failed: %w", err)
	}
	log.Infof("[Processor] Embedding 生成成功: md5=%s, chunks=%d, dims=%d, model=%s", task.FileMD5, len(esDocs), dims, p.embeddingCfg.Model)

	if err := p.esClient.BulkIndexDocuments(ctx, esDocs); err != nil {
		return fmt.Errorf("bulk index documents to elasticsearch failed: %w", err)
	}

	log.Infof("[Processor] Elasticsearch 索引成功: md5=%s, docs=%d, index=%s", task.FileMD5, len(esDocs), p.esClient.IndexName())
	log.Infof("[Processor] 文件处理成功完成: md5=%s", task.FileMD5)
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

func buildDocumentVectors(task tasks.FileProcessingTask, chunks []string, modelVersion string) []model.DocumentVector {
	vectors := make([]model.DocumentVector, 0, len(chunks))
	for i, chunk := range chunks {
		vectors = append(vectors, model.DocumentVector{
			FileMD5:      task.FileMD5,
			ChunkID:      i,
			TextContent:  chunk,
			ModelVersion: modelVersion,
			UserID:       task.UserID,
			OrgTag:       task.OrgTag,
			IsPublic:     task.IsPublic,
		})
	}
	return vectors
}

func (p *Processor) vectorizeDocuments(ctx context.Context, vectors []model.DocumentVector) ([]model.EsDocument, int, error) {
	esDocs := make([]model.EsDocument, 0, len(vectors))
	dimensions := 0

	for _, vector := range vectors {
		embeddingVector, err := p.embedding.CreateEmbedding(ctx, vector.TextContent)
		if err != nil {
			return nil, 0, fmt.Errorf("create embedding for chunk %d failed: %w", vector.ChunkID, err)
		}
		if p.embeddingCfg.Dimensions > 0 && len(embeddingVector) != p.embeddingCfg.Dimensions {
			return nil, 0, fmt.Errorf("embedding dimension mismatch for chunk %d: got=%d want=%d", vector.ChunkID, len(embeddingVector), p.embeddingCfg.Dimensions)
		}
		if dimensions == 0 {
			dimensions = len(embeddingVector)
		}

		esDocs = append(esDocs, buildEsDocument(vector, embeddingVector, p.embeddingCfg.Model))
	}

	return esDocs, dimensions, nil
}

func buildEsDocument(vector model.DocumentVector, embeddingVector []float32, defaultModelVersion string) model.EsDocument {
	modelVersion := strings.TrimSpace(vector.ModelVersion)
	if modelVersion == "" {
		modelVersion = defaultModelVersion
	}

	return model.EsDocument{
		VectorID:     model.BuildVectorID(vector.FileMD5, vector.ChunkID),
		FileMD5:      vector.FileMD5,
		ChunkID:      vector.ChunkID,
		TextContent:  vector.TextContent,
		Vector:       embeddingVector,
		ModelVersion: modelVersion,
		UserID:       vector.UserID,
		OrgTag:       vector.OrgTag,
		IsPublic:     vector.IsPublic,
	}
}
