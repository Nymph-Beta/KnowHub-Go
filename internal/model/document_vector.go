package model

import "time"

// DocumentVector 表示文档经过分块后的最小检索单元。
// 阶段九仅持久化文本与检索元数据，向量字段留到阶段十。
type DocumentVector struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	FileMD5      string    `gorm:"type:varchar(32);not null;index" json:"fileMd5"`
	ChunkID      int       `gorm:"not null" json:"chunkId"`
	TextContent  string    `gorm:"type:text;not null" json:"textContent"`
	ModelVersion string    `gorm:"type:varchar(100)" json:"modelVersion"`
	UserID       uint      `gorm:"not null;index" json:"userId"`
	OrgTag       string    `gorm:"type:varchar(50)" json:"orgTag"`
	IsPublic     bool      `gorm:"not null;default:false" json:"isPublic"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updatedAt"`
}

func (DocumentVector) TableName() string {
	return "document_vectors"
}
