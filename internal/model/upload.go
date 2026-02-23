package model

import "time"

// 文件上传相关，记录元数据和状态
type FileUpload struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	FileMD5   string    `gorm:"type:varchar(32);not null" json:"fileMd5"`
	FileName  string    `gorm:"type:varchar(255);not null" json:"fileName"`
	TotalSize int64     `gorm:"not null" json:"totalSize"`
	Status    int       `gorm:"type:tinyint;not null;default:0" json:"status"` // 0: 上传中, 1: 上传完成, 2: 上传失败
	UserID    uint      `gorm:"not null" json:"userId"`
	OrgTag    string    `gorm:"type:varchar(50)" json:"orgTag"`
	IsPublic  bool      `gorm:"not null;default:false" json:"isPublic"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updatedAt"`
}

func (FileUpload) TableName() string {
	return "file_uploads"
}
