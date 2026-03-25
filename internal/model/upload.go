package model

import "time"

const (
	FileUploadStatusUploading = 0
	FileUploadStatusUploaded  = 1
	FileUploadStatusFailed    = 2
)

const (
	FileProcessingStatusPending    = "pending"
	FileProcessingStatusProcessing = "processing"
	FileProcessingStatusIndexed    = "indexed"
	FileProcessingStatusEmpty      = "empty"
	FileProcessingStatusFailed     = "failed"
)

// 文件上传相关，记录元数据和状态
type FileUpload struct {
	ID               uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	FileMD5          string     `gorm:"type:varchar(32);not null" json:"fileMd5"`
	FileName         string     `gorm:"type:varchar(255);not null" json:"fileName"`
	TotalSize        int64      `gorm:"not null" json:"totalSize"`
	Status           int        `gorm:"type:tinyint;not null;default:0" json:"status"` // 0: 上传中, 1: 上传完成, 2: 上传失败
	ProcessingStatus string     `gorm:"type:varchar(32);not null;default:'pending'" json:"processingStatus"`
	UserID           uint       `gorm:"not null" json:"userId"`
	OrgTag           string     `gorm:"type:varchar(50)" json:"orgTag"`
	IsPublic         bool       `gorm:"not null;default:false" json:"isPublic"`
	MergedAt         *time.Time `gorm:"default:null" json:"mergedAt,omitempty"`
	CreatedAt        time.Time  `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt        time.Time  `gorm:"autoUpdateTime" json:"updatedAt"`
}

func (FileUpload) TableName() string {
	return "file_uploads"
}

// ChunkInfo 记录分片上传中每个分片的信息，与 FileUpload 通过 FileMD5 关联（1:N）
type ChunkInfo struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	FileMD5     string    `gorm:"type:varchar(32);not null;index" json:"fileMd5"`
	ChunkIndex  int       `gorm:"not null" json:"chunkIndex"`
	StoragePath string    `gorm:"type:varchar(500);not null" json:"storagePath"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"createdAt"`
}

func (ChunkInfo) TableName() string {
	return "chunk_infos"
}
