package repository

import (
	"fmt"
	"strings"

	"pai_smart_go_v2/internal/model"

	"gorm.io/gorm"
)

const defaultDocumentVectorBatchSize = 100

type DocumentVectorRepository interface {
	BatchCreate(vectors []model.DocumentVector) error
	FindByFileMD5(fileMD5 string) ([]model.DocumentVector, error)
	DeleteByFileMD5(fileMD5 string) error
}

type documentVectorRepository struct {
	db *gorm.DB
}

func NewDocumentVectorRepository(db *gorm.DB) DocumentVectorRepository {
	return &documentVectorRepository{db: db}
}

func (r *documentVectorRepository) BatchCreate(vectors []model.DocumentVector) error {
	if len(vectors) == 0 {
		return nil
	}
	return r.db.CreateInBatches(vectors, defaultDocumentVectorBatchSize).Error
}

func (r *documentVectorRepository) FindByFileMD5(fileMD5 string) ([]model.DocumentVector, error) {
	if strings.TrimSpace(fileMD5) == "" {
		return nil, fmt.Errorf("file_md5 is required")
	}

	var vectors []model.DocumentVector
	if err := r.db.Where("file_md5 = ?", fileMD5).Order("chunk_id ASC").Find(&vectors).Error; err != nil {
		return nil, err
	}
	return vectors, nil
}

func (r *documentVectorRepository) DeleteByFileMD5(fileMD5 string) error {
	if strings.TrimSpace(fileMD5) == "" {
		return fmt.Errorf("file_md5 is required")
	}
	return r.db.Where("file_md5 = ?", fileMD5).Delete(&model.DocumentVector{}).Error
}
