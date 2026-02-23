package repository

import (
	"fmt"
	"pai_smart_go_v2/internal/model"

	"gorm.io/gorm"
)

// 接口定义了文件上传数据的持久化操作。
// 实现：
// 创建文件上传记录
// 根据 MD5 + UserID 查询记录（用于秒传检查）
// 更新文件状态
type UploadRepository interface {
	Create(upload *model.FileUpload) error
	FindByFileMD5AndUserID(fileMD5 string, userID uint) (*model.FileUpload, error)
	FindByID(id uint) (*model.FileUpload, error)
}

// uploadRepository 是 UploadRepository 接口的 GORM 实现。
type uploadRepository struct {
	db *gorm.DB
}

func NewUploadRepository(db *gorm.DB) UploadRepository {
	return &uploadRepository{db: db}
}

// Create 创建一个新文件上传记录，上传成功后记录文件元信息
func (r *uploadRepository) Create(upload *model.FileUpload) error {
	if upload == nil {
		return fmt.Errorf("upload is nil")
	}
	return r.db.Create(upload).Error
}

// 上传前检查是否已上传过（秒传/去重）
func (r *uploadRepository) FindByFileMD5AndUserID(fileMD5 string, userID uint) (*model.FileUpload, error) {
	var upload model.FileUpload
	if err := r.db.Where("file_md5 = ? AND user_id = ?", fileMD5, userID).First(&upload).Error; err != nil {
		return nil, err
	}
	return &upload, nil
}

// 下载时根据 ID 查找文件记录
func (r *uploadRepository) FindByID(id uint) (*model.FileUpload, error) {
	var upload model.FileUpload
	if err := r.db.First(&upload, id).Error; err != nil {
		return nil, err
	}
	return &upload, nil
}
