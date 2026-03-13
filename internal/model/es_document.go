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

// SearchResponseDTO 表示返回给前端的检索结果。
type SearchResponseDTO struct {
	FileMD5     string  `json:"fileMd5"`
	FileName    string  `json:"fileName"`
	ChunkID     int     `json:"chunkId"`
	TextContent string  `json:"textContent"`
	Score       float64 `json:"score"`
	UserID      uint    `json:"userId"`
	OrgTag      string  `json:"orgTag"`
	IsPublic    bool    `json:"isPublic"`
}

func BuildVectorID(fileMD5 string, chunkID int) string {
	return fmt.Sprintf("%s_%d", fileMD5, chunkID)
}
