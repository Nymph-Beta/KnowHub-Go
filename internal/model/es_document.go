package model

import "fmt"

// EsDocument 表示写入 Elasticsearch 的检索文档。
type EsDocument struct {
	VectorID     string    `json:"vector_id"`
	FileMD5      string    `json:"file_md5"`
	ChunkID      int       `json:"chunk_id"`
	TextContent  string    `json:"text_content"`
	Vector       []float32 `json:"vector"`
	ModelVersion string    `json:"model_version"`
	UserID       uint      `json:"user_id"`
	OrgTag       string    `json:"org_tag"`
	IsPublic     bool      `json:"is_public"`
}

func BuildVectorID(fileMD5 string, chunkID int) string {
	return fmt.Sprintf("%s_%d", fileMD5, chunkID)
}
