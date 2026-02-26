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
	"strings"

	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/internal/repository"
	"pai_smart_go_v2/pkg/log"

	"github.com/minio/minio-go/v7"
	"gorm.io/gorm"
)

// 上传相关哨兵错误
var (
	// ErrFileAlreadyExists 文件已存在（同用户同MD5），用于秒传场景
	ErrFileAlreadyExists = errors.New("file already uploaded")
	// ErrFileNotFound 文件记录不存在
	ErrFileNotFound = errors.New("file not found")
	// ErrUploadFailed 上传到对象存储失败
	ErrUploadFailed = errors.New("upload to storage failed")
	// ErrUnsupportedFileType 文件类型不在 RAG 管线支持范围内
	ErrUnsupportedFileType = errors.New("unsupported file type")
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

// UploadService 定义文件上传域的业务接口
type UploadService interface {
	// SimpleUpload 简单上传：计算MD5 → 秒传检查 → 上传MinIO → 写DB
	SimpleUpload(ctx context.Context, userID uint, orgTag string, fileName string, fileSize int64, reader io.Reader) (*UploadResult, error)

	// DownloadFile 根据 fileMD5 + userID 查找文件记录，并从 MinIO 获取文件流。
	// 所有 MinIO 交互封装在 Service 内，保持 Handler 不直接接触存储层。
	DownloadFile(ctx context.Context, fileMD5 string, userID uint) (*DownloadResult, error)
}

// uploadService 是 UploadService 接口的具体实现
type uploadService struct {
	uploadRepo  repository.UploadRepository
	userRepo    repository.UserRepository
	minioClient *minio.Client
	bucketName  string
}

// NewUploadService 创建 UploadService 实例。
// 依赖注入 repository、MinIO 客户端和桶名，不依赖全局变量，便于单测 mock。
func NewUploadService(
	uploadRepo repository.UploadRepository,
	userRepo repository.UserRepository,
	minioClient *minio.Client,
	bucketName string,
) UploadService {
	return &uploadService{
		uploadRepo:  uploadRepo,
		userRepo:    userRepo,
		minioClient: minioClient,
		bucketName:  bucketName,
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
		FileMD5:   fileMD5,
		FileName:  fileName,
		TotalSize: int64(len(fileBytes)),
		Status:    1, // 简单上传一次完成，直接标记为"上传完成"
		UserID:    userID,
		OrgTag:    orgTag,
	}
	if err := s.uploadRepo.Create(upload); err != nil {
		log.Errorf("写入文件记录失败: %v", err)
		return nil, ErrInternal
	}

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

