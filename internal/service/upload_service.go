package service

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/internal/repository"
	"pai_smart_go_v2/pkg/log"
	"pai_smart_go_v2/pkg/tasks"

	"github.com/minio/minio-go/v7"
	"gorm.io/gorm"
)

const DefaultChunkSize int64 = 5 * 1024 * 1024 // 5MB

// 上传相关哨兵错误
var (
	ErrFileAlreadyExists   = errors.New("file already uploaded")
	ErrFileNotFound        = errors.New("file not found")
	ErrUploadFailed        = errors.New("upload to storage failed")
	ErrUnsupportedFileType = errors.New("unsupported file type")
	ErrChunksIncomplete    = errors.New("not all chunks uploaded")
	ErrMergeFailed         = errors.New("merge chunks failed")
)

// allowedExtensions RAG 系统支持处理的文件扩展名白名单
var allowedExtensions = map[string]bool{
	".pdf": true, ".docx": true, ".doc": true,
	".txt": true, ".md": true, ".csv": true,
	".xlsx": true, ".xls": true, ".pptx": true,
}

// UploadResult 上传成功后返回给 Handler 的结果
type UploadResult struct {
	FileMD5   string `json:"fileMd5"`
	FileName  string `json:"fileName"`
	TotalSize int64  `json:"totalSize"`
	IsQuick   bool   `json:"isQuick"` // 是否秒传（已存在相同文件）
}

// DownloadResult 封装下载所需的全部信息，Handler 只需做 HTTP 响应转换。
// 调用方需要在使用完后关闭 Reader。
type DownloadResult struct {
	FileName    string
	ContentType string
	Size        int64
	Reader      io.ReadCloser
}

// CheckResult 文件检查结果，用于秒传和断点续传
type CheckResult struct {
	Completed      bool  `json:"completed"`
	UploadedChunks []int `json:"uploadedChunks"`
}

// ChunkUploadResult 单个分片上传后返回的进度信息
type ChunkUploadResult struct {
	UploadedChunks []int   `json:"uploadedChunks"`
	Progress       float64 `json:"progress"`
}

type UploadStatusResult struct {
	FileMD5          string  `json:"fileMd5"`
	Status           int     `json:"status"`
	ProcessingStatus string  `json:"processingStatus"`
	Completed        bool    `json:"completed"`
	UploadedChunks   []int   `json:"uploadedChunks"`
	Progress         float64 `json:"progress"`
}

type FastUploadCheckResult struct {
	CanQuickUpload   bool   `json:"canQuickUpload"`
	FileMD5          string `json:"fileMd5,omitempty"`
	FileName         string `json:"fileName,omitempty"`
	TotalSize        int64  `json:"totalSize,omitempty"`
	Status           int    `json:"status,omitempty"`
	ProcessingStatus string `json:"processingStatus,omitempty"`
}

// MergeResult 分片合并后返回的文件信息
type MergeResult struct {
	ObjectURL string `json:"objectUrl"`
	FileMD5   string `json:"fileMd5"`
	FileName  string `json:"fileName"`
}

// UploadService 定义文件上传域的业务接口
type UploadService interface {
	// SimpleUpload 简单上传：计算MD5 → 秒传检查 → 上传MinIO → 写DB（阶段六）
	SimpleUpload(ctx context.Context, userID uint, orgTag string, fileName string, fileSize int64, reader io.Reader) (*UploadResult, error)

	// DownloadFile 根据 fileMD5 + userID 查找文件记录，并从 MinIO 获取文件流。
	DownloadFile(ctx context.Context, fileMD5 string, userID uint) (*DownloadResult, error)

	// CheckFile 检查文件上传状态：秒传判断 + 已上传分片列表（阶段七）
	CheckFile(ctx context.Context, fileMD5 string, userID uint) (*CheckResult, error)

	// GetUploadStatus 返回当前文件的上传进度，兼容原始前端轮询接口。
	GetUploadStatus(ctx context.Context, fileMD5 string, userID uint) (*UploadStatusResult, error)

	// CheckFastUpload 返回秒传检查结果，兼容原始前端上传链路。
	CheckFastUpload(ctx context.Context, fileMD5 string, userID uint) (*FastUploadCheckResult, error)

	// GetSupportedTypes 返回允许上传的扩展名列表。
	GetSupportedTypes() []string

	// UploadChunk 上传单个分片到 MinIO 并在 Redis bitmap 中标记
	UploadChunk(ctx context.Context, fileMD5 string, fileName string, totalSize int64, chunkIndex int, reader io.Reader, chunkSize int64, userID uint, orgTag string, isPublic bool) (*ChunkUploadResult, error)

	// MergeChunks 合并所有分片为最终文件，更新状态，异步清理临时数据
	MergeChunks(ctx context.Context, fileMD5 string, fileName string, userID uint) (*MergeResult, error)
}

// TaskProducer 抽象了文件处理任务的异步投递能力（如 Kafka producer）。
type TaskProducer interface {
	ProduceFileTask(ctx context.Context, task tasks.FileProcessingTask) error
}

// uploadService 是 UploadService 接口的具体实现
type uploadService struct {
	uploadRepo   repository.UploadRepository
	userRepo     repository.UserRepository
	minioClient  *minio.Client
	bucketName   string
	taskProducer TaskProducer
}

// NewUploadService 创建 UploadService 实例。
// 依赖注入 repository、MinIO 客户端和桶名，不依赖全局变量，便于单测 mock。
func NewUploadService(
	uploadRepo repository.UploadRepository,
	userRepo repository.UserRepository,
	minioClient *minio.Client,
	bucketName string,
	taskProducer TaskProducer,
) UploadService {
	return &uploadService{
		uploadRepo:   uploadRepo,
		userRepo:     userRepo,
		minioClient:  minioClient,
		bucketName:   bucketName,
		taskProducer: taskProducer,
	}
}

// SimpleUpload 实现简单文件上传的完整流程：
//  1. 读取文件内容并计算 MD5
//  2. 检查是否已上传过（秒传）
//  3. 上传到 MinIO 对象存储
//  4. 写入数据库记录
func (s *uploadService) SimpleUpload(
	ctx context.Context,
	userID uint,
	orgTag string,
	fileName string,
	fileSize int64,
	reader io.Reader,
) (*UploadResult, error) {
	// 0a. 文件扩展名校验：只接受 RAG 管线能处理的格式，提前拦截无效上传
	ext := strings.ToLower(filepath.Ext(fileName))
	if !allowedExtensions[ext] {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedFileType, ext)
	}

	// 0b. OrgTag 为空时，用用户的 PrimaryOrg 回填，避免后续权限过滤时遗漏
	if orgTag == "" {
		user, userErr := s.userRepo.FindByID(userID)
		if userErr != nil {
			log.Errorf("查询用户信息失败: %v", userErr)
			return nil, ErrInternal
		}
		orgTag = user.PrimaryOrg
	}

	// 1. 使用 TeeReader 实现单次遍历同时完成读取和 MD5 计算：
	//    ReadAll(teeReader) 每读一块数据，TeeReader 会自动写入 hasher，
	//    读完后 hasher 已累计完整文件的哈希，无需二次遍历。
	//    因为秒传检查需要先拿到 MD5 才能决定是否上传，所以必须先读完整文件。
	hasher := md5.New()
	teeReader := io.TeeReader(reader, hasher)
	fileBytes, err := io.ReadAll(teeReader)
	if err != nil {
		log.Errorf("读取文件内容失败: %v", err)
		return nil, fmt.Errorf("%w: %v", ErrUploadFailed, err)
	}
	fileMD5 := hex.EncodeToString(hasher.Sum(nil))

	// 2. 秒传检查：同用户 + 同 MD5 表示已上传过
	existing, err := s.uploadRepo.FindByFileMD5AndUserID(fileMD5, userID)
	if err == nil && existing != nil {
		log.Infof("秒传命中: user=%d, md5=%s, file=%s", userID, fileMD5, fileName)
		return &UploadResult{
			FileMD5:   existing.FileMD5,
			FileName:  existing.FileName,
			TotalSize: existing.TotalSize,
			IsQuick:   true,
		}, nil
	}
	// 非 "record not found" 的错误才需要处理
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Errorf("查询文件记录失败: %v", err)
		return nil, ErrInternal
	}

	// 3. 上传到 MinIO
	// 对象键格式：uploads/<userID>/<md5>/<原始文件名>
	objectKey := fmt.Sprintf("uploads/%d/%s/%s", userID, fileMD5, fileName)

	_, err = s.minioClient.PutObject(
		ctx,
		s.bucketName,
		objectKey,
		bytes.NewReader(fileBytes),
		int64(len(fileBytes)),
		minio.PutObjectOptions{
			ContentType: "application/octet-stream",
		},
	)
	if err != nil {
		log.Errorf("上传文件到 MinIO 失败: %v", err)
		return nil, fmt.Errorf("%w: %v", ErrUploadFailed, err)
	}

	log.Infof("文件上传 MinIO 成功: bucket=%s, key=%s", s.bucketName, objectKey)

	// 4. 写入数据库记录
	upload := &model.FileUpload{
		FileMD5:          fileMD5,
		FileName:         fileName,
		TotalSize:        int64(len(fileBytes)),
		Status:           model.FileUploadStatusUploaded, // 简单上传一次完成，直接标记为"上传完成"
		ProcessingStatus: model.FileProcessingStatusPending,
		UserID:           userID,
		OrgTag:           orgTag,
	}
	if err := s.uploadRepo.Create(upload); err != nil {
		log.Errorf("写入文件记录失败: %v", err)
		return nil, ErrInternal
	}
	s.produceFileTask(ctx, upload, objectKey)

	return &UploadResult{
		FileMD5:   fileMD5,
		FileName:  fileName,
		TotalSize: int64(len(fileBytes)),
		IsQuick:   false,
	}, nil
}

// DownloadFile 查找文件记录并从 MinIO 获取对象流。
// 保持所有 MinIO 交互在 Service 内，Handler 不直接依赖 storage 全局变量。
func (s *uploadService) DownloadFile(ctx context.Context, fileMD5 string, userID uint) (*DownloadResult, error) {
	// 1. 查找文件记录
	upload, err := s.uploadRepo.FindByFileMD5AndUserID(fileMD5, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrFileNotFound
		}
		log.Errorf("查询文件记录失败: %v", err)
		return nil, ErrInternal
	}

	// 2. 从 MinIO 获取对象流
	objectKey := fmt.Sprintf("uploads/%d/%s/%s", upload.UserID, upload.FileMD5, upload.FileName)
	object, err := s.minioClient.GetObject(ctx, s.bucketName, objectKey, minio.GetObjectOptions{})
	if err != nil {
		log.Errorf("从 MinIO 获取文件失败: %v", err)
		return nil, ErrInternal
	}

	// 3. 获取对象元信息（大小、ContentType）
	objectInfo, err := object.Stat()
	if err != nil {
		object.Close()
		log.Errorf("获取 MinIO 对象信息失败: %v", err)
		return nil, ErrInternal
	}

	return &DownloadResult{
		FileName:    upload.FileName,
		ContentType: objectInfo.ContentType,
		Size:        objectInfo.Size,
		Reader:      object,
	}, nil
}

// ========== 阶段七：分片上传 ==========

func calcTotalChunks(totalSize int64) int {
	return int((totalSize + DefaultChunkSize - 1) / DefaultChunkSize)
}

// CheckFile 检查文件上传状态：
//  1. status=1 → 已完成（秒传命中）
//  2. status=0 → 上传中，返回已上传分片列表（断点续传）
//  3. 无记录 → 全新上传
func (s *uploadService) CheckFile(ctx context.Context, fileMD5 string, userID uint) (*CheckResult, error) {
	existing, err := s.uploadRepo.FindByFileMD5AndUserID(fileMD5, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &CheckResult{Completed: false, UploadedChunks: []int{}}, nil
		}
		log.Errorf("CheckFile: 查询文件记录失败: %v", err)
		return nil, ErrInternal
	}

	if existing.Status == 1 {
		return &CheckResult{Completed: true, UploadedChunks: []int{}}, nil
	}

	totalChunks := calcTotalChunks(existing.TotalSize)
	uploadedChunks, err := s.uploadRepo.GetUploadedChunksFromRedis(ctx, fileMD5, userID, totalChunks)
	if err != nil {
		log.Errorf("CheckFile: 读取 Redis bitmap 失败: %v", err)
		return nil, ErrInternal
	}

	return &CheckResult{Completed: false, UploadedChunks: uploadedChunks}, nil
}

func (s *uploadService) GetUploadStatus(ctx context.Context, fileMD5 string, userID uint) (*UploadStatusResult, error) {
	fileMD5 = strings.TrimSpace(fileMD5)
	if fileMD5 == "" || userID == 0 {
		return nil, ErrInvalidInput
	}

	existing, err := s.uploadRepo.FindByFileMD5AndUserID(fileMD5, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrFileNotFound
		}
		log.Errorf("GetUploadStatus: 查询文件记录失败: %v", err)
		return nil, ErrInternal
	}

	result := &UploadStatusResult{
		FileMD5:          existing.FileMD5,
		Status:           existing.Status,
		ProcessingStatus: existing.ProcessingStatus,
		Completed:        existing.Status == model.FileUploadStatusUploaded,
		UploadedChunks:   []int{},
		Progress:         0,
	}
	if existing.Status == model.FileUploadStatusUploaded {
		result.Progress = 100
		return result, nil
	}

	totalChunks := calcTotalChunks(existing.TotalSize)
	uploadedChunks, err := s.uploadRepo.GetUploadedChunksFromRedis(ctx, fileMD5, userID, totalChunks)
	if err != nil {
		log.Errorf("GetUploadStatus: 读取 Redis bitmap 失败: %v", err)
		return nil, ErrInternal
	}
	result.UploadedChunks = uploadedChunks
	if totalChunks > 0 {
		result.Progress = float64(len(uploadedChunks)) / float64(totalChunks) * 100
	}
	return result, nil
}

func (s *uploadService) CheckFastUpload(ctx context.Context, fileMD5 string, userID uint) (*FastUploadCheckResult, error) {
	fileMD5 = strings.TrimSpace(fileMD5)
	if fileMD5 == "" || userID == 0 {
		return nil, ErrInvalidInput
	}

	existing, err := s.uploadRepo.FindByFileMD5AndUserID(fileMD5, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &FastUploadCheckResult{CanQuickUpload: false}, nil
		}
		log.Errorf("CheckFastUpload: 查询文件记录失败: %v", err)
		return nil, ErrInternal
	}

	return &FastUploadCheckResult{
		CanQuickUpload:   existing.Status == model.FileUploadStatusUploaded,
		FileMD5:          existing.FileMD5,
		FileName:         existing.FileName,
		TotalSize:        existing.TotalSize,
		Status:           existing.Status,
		ProcessingStatus: existing.ProcessingStatus,
	}, nil
}

func (s *uploadService) GetSupportedTypes() []string {
	types := make([]string, 0, len(allowedExtensions))
	for ext := range allowedExtensions {
		types = append(types, ext)
	}
	sort.Strings(types)
	return types
}

// UploadChunk 上传单个分片：
//  1. FindOrCreate FileUpload 记录
//  2. 幂等检查（GETBIT）→ 已上传则跳过
//  3. PutObject 到 chunks/{fileMD5}/{chunkIndex}
//  4. 创建 ChunkInfo + SETBIT
//  5. 返回当前进度
func (s *uploadService) UploadChunk(
	ctx context.Context,
	fileMD5, fileName string,
	totalSize int64,
	chunkIndex int,
	reader io.Reader,
	chunkSize int64,
	userID uint,
	orgTag string,
	isPublic bool,
) (*ChunkUploadResult, error) {
	ext := strings.ToLower(filepath.Ext(fileName))
	if !allowedExtensions[ext] {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedFileType, ext)
	}

	if orgTag == "" {
		user, err := s.userRepo.FindByID(userID)
		if err != nil {
			log.Errorf("UploadChunk: 查询用户信息失败: %v", err)
			return nil, ErrInternal
		}
		orgTag = user.PrimaryOrg
	}

	totalChunks := calcTotalChunks(totalSize)

	// FindOrCreate FileUpload
	existing, err := s.uploadRepo.FindByFileMD5AndUserID(fileMD5, userID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Errorf("UploadChunk: 查询文件记录失败: %v", err)
			return nil, ErrInternal
		}
		upload := &model.FileUpload{
			FileMD5:          fileMD5,
			FileName:         fileName,
			TotalSize:        totalSize,
			Status:           model.FileUploadStatusUploading,
			ProcessingStatus: model.FileProcessingStatusPending,
			UserID:           userID,
			OrgTag:           orgTag,
			IsPublic:         isPublic,
		}
		if createErr := s.uploadRepo.Create(upload); createErr != nil {
			log.Errorf("UploadChunk: 创建文件记录失败: %v", createErr)
			return nil, ErrInternal
		}
	} else if existing.Status == model.FileUploadStatusUploaded {
		uploadedChunks := makeRange(totalChunks)
		return &ChunkUploadResult{UploadedChunks: uploadedChunks, Progress: 100}, nil
	}

	// Idempotency: skip if already uploaded
	alreadyUploaded, err := s.uploadRepo.IsChunkUploaded(ctx, fileMD5, userID, chunkIndex)
	if err != nil {
		log.Errorf("UploadChunk: 检查分片状态失败: %v", err)
		return nil, ErrInternal
	}
	if alreadyUploaded {
		log.Infof("UploadChunk: 分片已存在, md5=%s, index=%d", fileMD5, chunkIndex)
		uploadedChunks, _ := s.uploadRepo.GetUploadedChunksFromRedis(ctx, fileMD5, userID, totalChunks)
		return &ChunkUploadResult{
			UploadedChunks: uploadedChunks,
			Progress:       float64(len(uploadedChunks)) / float64(totalChunks) * 100,
		}, nil
	}

	// Upload chunk to MinIO
	chunkKey := fmt.Sprintf("chunks/%s/%d", fileMD5, chunkIndex)
	_, err = s.minioClient.PutObject(ctx, s.bucketName, chunkKey, reader, chunkSize, minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	if err != nil {
		log.Errorf("UploadChunk: 上传分片到 MinIO 失败: %v", err)
		return nil, fmt.Errorf("%w: %v", ErrUploadFailed, err)
	}

	// Create ChunkInfo DB record
	chunkInfo := &model.ChunkInfo{
		FileMD5:     fileMD5,
		ChunkIndex:  chunkIndex,
		StoragePath: chunkKey,
	}
	if err := s.uploadRepo.CreateChunkInfo(chunkInfo); err != nil {
		log.Errorf("UploadChunk: 创建分片记录失败: %v", err)
		return nil, ErrInternal
	}

	// Mark in Redis bitmap
	if err := s.uploadRepo.MarkChunkUploaded(ctx, fileMD5, userID, chunkIndex); err != nil {
		log.Errorf("UploadChunk: 标记分片失败: %v", err)
		return nil, ErrInternal
	}

	uploadedChunks, _ := s.uploadRepo.GetUploadedChunksFromRedis(ctx, fileMD5, userID, totalChunks)
	return &ChunkUploadResult{
		UploadedChunks: uploadedChunks,
		Progress:       float64(len(uploadedChunks)) / float64(totalChunks) * 100,
	}, nil
}

// MergeChunks 合并所有分片为最终文件：
//  1. 检查所有分片是否已上传
//  2. 单分片 CopyObject / 多分片 ComposeObject
//  3. 更新 DB 状态
//  4. 异步清理 Redis + MinIO 临时分片
func (s *uploadService) MergeChunks(ctx context.Context, fileMD5, fileName string, userID uint) (*MergeResult, error) {
	upload, err := s.uploadRepo.FindByFileMD5AndUserID(fileMD5, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrFileNotFound
		}
		log.Errorf("MergeChunks: 查询文件记录失败: %v", err)
		return nil, ErrInternal
	}

	destKey := fmt.Sprintf("uploads/%d/%s/%s", userID, fileMD5, fileName)

	if upload.Status == model.FileUploadStatusUploaded {
		return &MergeResult{ObjectURL: destKey, FileMD5: fileMD5, FileName: fileName}, nil
	}

	totalChunks := calcTotalChunks(upload.TotalSize)

	uploadedChunks, err := s.uploadRepo.GetUploadedChunksFromRedis(ctx, fileMD5, userID, totalChunks)
	if err != nil {
		log.Errorf("MergeChunks: 获取上传分片列表失败: %v", err)
		return nil, ErrInternal
	}
	if len(uploadedChunks) != totalChunks {
		log.Errorf("MergeChunks: 分片未完整, uploaded=%d, total=%d", len(uploadedChunks), totalChunks)
		return nil, ErrChunksIncomplete
	}

	// MinIO merge: single chunk uses CopyObject, multiple uses ComposeObject
	dst := minio.CopyDestOptions{
		Bucket: s.bucketName,
		Object: destKey,
	}

	if totalChunks == 1 {
		src := minio.CopySrcOptions{
			Bucket: s.bucketName,
			Object: fmt.Sprintf("chunks/%s/0", fileMD5),
		}
		_, err = s.minioClient.CopyObject(ctx, dst, src)
	} else {
		srcs := make([]minio.CopySrcOptions, totalChunks)
		for i := 0; i < totalChunks; i++ {
			srcs[i] = minio.CopySrcOptions{
				Bucket: s.bucketName,
				Object: fmt.Sprintf("chunks/%s/%d", fileMD5, i),
			}
		}
		_, err = s.minioClient.ComposeObject(ctx, dst, srcs...)
	}
	if err != nil {
		log.Errorf("MergeChunks: MinIO 合并失败: %v", err)
		return nil, fmt.Errorf("%w: %v", ErrMergeFailed, err)
	}

	log.Infof("MergeChunks: 文件合并成功: %s", destKey)

	now := time.Now()
	if err := s.uploadRepo.UpdateFileUploadStatus(fileMD5, userID, model.FileUploadStatusUploaded, &now); err != nil {
		log.Errorf("MergeChunks: 更新文件状态失败: %v", err)
		return nil, ErrInternal
	}
	if err := s.uploadRepo.UpdateFileProcessingStatus(fileMD5, userID, model.FileProcessingStatusPending); err != nil {
		log.Errorf("MergeChunks: 更新处理状态失败: %v", err)
		return nil, ErrInternal
	}
	s.produceFileTask(ctx, upload, destKey)

	go s.cleanupAfterMerge(fileMD5, userID, totalChunks)

	return &MergeResult{ObjectURL: destKey, FileMD5: fileMD5, FileName: fileName}, nil
}

// cleanupAfterMerge 异步删除 Redis bitmap 和 MinIO 临时分片对象
func (s *uploadService) cleanupAfterMerge(fileMD5 string, userID uint, totalChunks int) {
	ctx := context.Background()

	if err := s.uploadRepo.DeleteUploadMark(ctx, fileMD5, userID); err != nil {
		log.Errorf("cleanupAfterMerge: 删除 Redis bitmap 失败: %v", err)
	}

	for i := 0; i < totalChunks; i++ {
		chunkKey := fmt.Sprintf("chunks/%s/%d", fileMD5, i)
		if err := s.minioClient.RemoveObject(ctx, s.bucketName, chunkKey, minio.RemoveObjectOptions{}); err != nil {
			log.Errorf("cleanupAfterMerge: 删除 MinIO 分片 %s 失败: %v", chunkKey, err)
		}
	}

	log.Infof("cleanupAfterMerge: 清理完成, md5=%s, user=%d", fileMD5, userID)
}

// makeRange 生成 [0, n) 的整数切片
func makeRange(n int) []int {
	result := make([]int, n)
	for i := 0; i < n; i++ {
		result[i] = i
	}
	return result
}

func (s *uploadService) produceFileTask(ctx context.Context, upload *model.FileUpload, objectKey string) {
	markFailed := func() {
		if s.uploadRepo == nil || upload == nil {
			return
		}
		_ = s.uploadRepo.UpdateFileProcessingStatus(upload.FileMD5, upload.UserID, model.FileProcessingStatusFailed)
	}

	if s.taskProducer == nil || upload == nil {
		markFailed()
		return
	}

	task := tasks.FileProcessingTask{
		FileMD5:   upload.FileMD5,
		FileName:  upload.FileName,
		UserID:    upload.UserID,
		OrgTag:    upload.OrgTag,
		IsPublic:  upload.IsPublic,
		ObjectKey: objectKey,
	}

	if err := s.taskProducer.ProduceFileTask(ctx, task); err != nil {
		log.Errorf("发送文件处理任务失败(不影响主流程): md5=%s err=%v", upload.FileMD5, err)
		markFailed()
		return
	}
	log.Infof("文件处理任务已投递: md5=%s", upload.FileMD5)
}
