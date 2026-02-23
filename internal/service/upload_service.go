package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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
)

// UploadResult 上传成功后返回给 Handler 的结果
type UploadResult struct {
	FileMD5   string `json:"fileMd5"`
	FileName  string `json:"fileName"`
	TotalSize int64  `json:"totalSize"`
	IsQuick   bool   `json:"isQuick"` // 是否秒传（已存在相同文件）
}

// UploadService 定义文件上传域的业务接口
type UploadService interface {
	// SimpleUpload 简单上传：计算MD5 → 秒传检查 → 上传MinIO → 写DB
	SimpleUpload(ctx context.Context, userID uint, orgTag string, fileName string, fileSize int64, reader io.Reader) (*UploadResult, error)

	// GetFileForDownload 根据 fileMD5 + userID 查找文件记录（供下载用）
	GetFileForDownload(ctx context.Context, fileMD5 string, userID uint) (*model.FileUpload, error)
}

// uploadService 是 UploadService 接口的具体实现
type uploadService struct {
	uploadRepo  repository.UploadRepository
	minioClient *minio.Client
	bucketName  string
}

// NewUploadService 创建 UploadService 实例。
// 依赖注入 repository、MinIO 客户端和桶名，不依赖全局变量，便于单测 mock。
func NewUploadService(
	uploadRepo repository.UploadRepository,
	minioClient *minio.Client,
	bucketName string,
) UploadService {
	return &uploadService{
		uploadRepo:  uploadRepo,
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
	// 1. 边读边算 MD5（避免把整个文件加载到内存再算一遍）
	hasher := md5.New()
	teeReader := io.TeeReader(reader, hasher)

	// 先把文件内容读到一个 buffer，同时完成 MD5 计算
	// 因为 MinIO PutObject 需要 reader，而我们需要先算完 MD5 才能判断秒传
	// 所以用 TeeReader + Pipe 的方式：先全部读完算 MD5，再上传
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
		io.NopCloser(newBytesReader(fileBytes)),
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

// GetFileForDownload 根据 fileMD5 和 userID 查找文件记录。
// 找不到时返回 ErrFileNotFound，供 Handler 映射为 404。
func (s *uploadService) GetFileForDownload(ctx context.Context, fileMD5 string, userID uint) (*model.FileUpload, error) {
	upload, err := s.uploadRepo.FindByFileMD5AndUserID(fileMD5, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrFileNotFound
		}
		log.Errorf("查询文件记录失败: %v", err)
		return nil, ErrInternal
	}
	return upload, nil
}

// bytesReader 是一个简单的 bytes.Reader 包装，避免引入额外的 buffer 包
type bytesReader struct {
	data   []byte
	offset int
}

func newBytesReader(data []byte) *bytesReader {
	return &bytesReader{data: data}
}

func (r *bytesReader) Read(p []byte) (n int, err error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}
