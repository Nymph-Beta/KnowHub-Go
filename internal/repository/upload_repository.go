package repository

import (
	"context"
	"fmt"
	"pai_smart_go_v2/internal/model"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// UploadRepository 定义文件上传数据的持久化操作（GORM + Redis）。
type UploadRepository interface {
	// --- GORM: FileUpload ---
	Create(upload *model.FileUpload) error
	FindByFileMD5AndUserID(fileMD5 string, userID uint) (*model.FileUpload, error)
	FindBatchByMD5s(fileMD5s []string) ([]model.FileUpload, error)
	FindFilesByUserID(userID uint) ([]model.FileUpload, error)
	FindAccessibleFiles(userID uint, orgTags []string) ([]model.FileUpload, error)
	FindAccessibleFileByMD5(userID uint, orgTags []string, fileMD5 string) (*model.FileUpload, error)
	FindAccessibleFilesByName(userID uint, orgTags []string, fileName string) ([]model.FileUpload, error)
	FindByID(id uint) (*model.FileUpload, error)
	DeleteFileUploadRecord(fileMD5 string, userID uint) error
	UpdateFileUploadStatus(fileMD5 string, userID uint, status int, mergedAt *time.Time) error
	UpdateFileProcessingStatus(fileMD5 string, userID uint, processingStatus string) error

	// --- GORM: ChunkInfo ---
	CreateChunkInfo(chunk *model.ChunkInfo) error
	FindChunksByFileMD5(fileMD5 string) ([]model.ChunkInfo, error)
	DeleteChunkInfosByFileMD5(fileMD5 string) error

	// --- Redis Bitmap ---
	IsChunkUploaded(ctx context.Context, fileMD5 string, userID uint, chunkIndex int) (bool, error)
	MarkChunkUploaded(ctx context.Context, fileMD5 string, userID uint, chunkIndex int) error
	GetUploadedChunksFromRedis(ctx context.Context, fileMD5 string, userID uint, totalChunks int) ([]int, error)
	DeleteUploadMark(ctx context.Context, fileMD5 string, userID uint) error
}

type uploadRepository struct {
	db  *gorm.DB
	rdb *redis.Client
}

func NewUploadRepository(db *gorm.DB, rdb *redis.Client) UploadRepository {
	return &uploadRepository{db: db, rdb: rdb}
}

func uploadBitmapKey(userID uint, fileMD5 string) string {
	return fmt.Sprintf("upload:%d:%s", userID, fileMD5)
}

// ========== GORM: FileUpload ==========

func (r *uploadRepository) Create(upload *model.FileUpload) error {
	if upload == nil {
		return fmt.Errorf("upload is nil")
	}
	return r.db.Create(upload).Error
}

func (r *uploadRepository) FindByFileMD5AndUserID(fileMD5 string, userID uint) (*model.FileUpload, error) {
	var upload model.FileUpload
	if err := r.db.Where("file_md5 = ? AND user_id = ?", fileMD5, userID).First(&upload).Error; err != nil {
		return nil, err
	}
	return &upload, nil
}

func (r *uploadRepository) FindBatchByMD5s(fileMD5s []string) ([]model.FileUpload, error) {
	if len(fileMD5s) == 0 {
		return []model.FileUpload{}, nil
	}

	var uploads []model.FileUpload
	if err := r.db.Where("file_md5 IN ?", fileMD5s).Find(&uploads).Error; err != nil {
		return nil, err
	}
	return uploads, nil
}

func (r *uploadRepository) FindFilesByUserID(userID uint) ([]model.FileUpload, error) {
	var uploads []model.FileUpload
	if err := r.db.Where("user_id = ? AND status = ?", userID, 1).
		Order("created_at DESC").
		Find(&uploads).Error; err != nil {
		return nil, err
	}
	return uploads, nil
}

func (r *uploadRepository) FindAccessibleFiles(userID uint, orgTags []string) ([]model.FileUpload, error) {
	var uploads []model.FileUpload
	if err := r.buildAccessibleFilesQuery(userID, orgTags).
		Order("created_at DESC").
		Find(&uploads).Error; err != nil {
		return nil, err
	}
	return uploads, nil
}

func (r *uploadRepository) FindAccessibleFileByMD5(userID uint, orgTags []string, fileMD5 string) (*model.FileUpload, error) {
	var upload model.FileUpload
	if err := r.buildAccessibleFilesQuery(userID, orgTags).
		Where("file_md5 = ?", fileMD5).
		First(&upload).Error; err != nil {
		return nil, err
	}
	return &upload, nil
}

func (r *uploadRepository) FindAccessibleFilesByName(userID uint, orgTags []string, fileName string) ([]model.FileUpload, error) {
	var uploads []model.FileUpload
	if err := r.buildAccessibleFilesQuery(userID, orgTags).
		Where("file_name = ?", fileName).
		Order("created_at DESC").
		Find(&uploads).Error; err != nil {
		return nil, err
	}
	return uploads, nil
}

func (r *uploadRepository) FindByID(id uint) (*model.FileUpload, error) {
	var upload model.FileUpload
	if err := r.db.First(&upload, id).Error; err != nil {
		return nil, err
	}
	return &upload, nil
}

func (r *uploadRepository) DeleteFileUploadRecord(fileMD5 string, userID uint) error {
	return r.db.Where("file_md5 = ? AND user_id = ?", fileMD5, userID).
		Delete(&model.FileUpload{}).Error
}

func (r *uploadRepository) UpdateFileUploadStatus(fileMD5 string, userID uint, status int, mergedAt *time.Time) error {
	updates := map[string]interface{}{"status": status}
	if mergedAt != nil {
		updates["merged_at"] = mergedAt
	}
	return r.db.Model(&model.FileUpload{}).
		Where("file_md5 = ? AND user_id = ?", fileMD5, userID).
		Updates(updates).Error
}

func (r *uploadRepository) UpdateFileProcessingStatus(fileMD5 string, userID uint, processingStatus string) error {
	return r.db.Model(&model.FileUpload{}).
		Where("file_md5 = ? AND user_id = ?", fileMD5, userID).
		Update("processing_status", processingStatus).Error
}

// ========== GORM: ChunkInfo ==========

func (r *uploadRepository) CreateChunkInfo(chunk *model.ChunkInfo) error {
	if chunk == nil {
		return fmt.Errorf("chunk is nil")
	}
	return r.db.Create(chunk).Error
}

func (r *uploadRepository) FindChunksByFileMD5(fileMD5 string) ([]model.ChunkInfo, error) {
	var chunks []model.ChunkInfo
	if err := r.db.Where("file_md5 = ?", fileMD5).Order("chunk_index ASC").Find(&chunks).Error; err != nil {
		return nil, err
	}
	return chunks, nil
}

func (r *uploadRepository) DeleteChunkInfosByFileMD5(fileMD5 string) error {
	return r.db.Where("file_md5 = ?", fileMD5).Delete(&model.ChunkInfo{}).Error
}

// ========== Redis Bitmap ==========

func (r *uploadRepository) IsChunkUploaded(ctx context.Context, fileMD5 string, userID uint, chunkIndex int) (bool, error) {
	key := uploadBitmapKey(userID, fileMD5)
	val, err := r.rdb.GetBit(ctx, key, int64(chunkIndex)).Result()
	if err != nil {
		return false, err
	}
	return val == 1, nil
}

func (r *uploadRepository) MarkChunkUploaded(ctx context.Context, fileMD5 string, userID uint, chunkIndex int) error {
	key := uploadBitmapKey(userID, fileMD5)
	return r.rdb.SetBit(ctx, key, int64(chunkIndex), 1).Err()
}

// GetUploadedChunksFromRedis 通过读取 bitmap 原始字节解析出已上传分片索引列表。
// Redis bitmap 每个字节的最高位对应最小 offset：bit 0 = byte[0] 的 bit7。
func (r *uploadRepository) GetUploadedChunksFromRedis(ctx context.Context, fileMD5 string, userID uint, totalChunks int) ([]int, error) {
	key := uploadBitmapKey(userID, fileMD5)
	data, err := r.rdb.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return []int{}, nil
	}
	if err != nil {
		return nil, err
	}

	uploaded := make([]int, 0)
	for i := 0; i < totalChunks; i++ {
		byteIdx := i / 8
		bitIdx := 7 - (i % 8) // Redis big-endian bit ordering within each byte
		if byteIdx < len(data) && (data[byteIdx]>>uint(bitIdx))&1 == 1 {
			uploaded = append(uploaded, i)
		}
	}
	return uploaded, nil
}

func (r *uploadRepository) DeleteUploadMark(ctx context.Context, fileMD5 string, userID uint) error {
	key := uploadBitmapKey(userID, fileMD5)
	return r.rdb.Del(ctx, key).Err()
}

func (r *uploadRepository) buildAccessibleFilesQuery(userID uint, orgTags []string) *gorm.DB {
	base := r.db.Model(&model.FileUpload{}).Where("status = ?", 1)
	permission := r.db.Where("user_id = ?", userID).Or("is_public = ?", true)
	if len(orgTags) > 0 {
		permission = permission.Or("org_tag IN ?", orgTags)
	}
	return base.Where(permission)
}
