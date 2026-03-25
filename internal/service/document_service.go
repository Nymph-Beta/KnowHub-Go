package service

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/internal/repository"
	"pai_smart_go_v2/pkg/log"

	"github.com/minio/minio-go/v7"
)

type FileUploadDTO struct {
	model.FileUpload
	OrgTagName string `json:"orgTagName"`
}

type DownloadInfoDTO struct {
	FileMD5     string `json:"fileMd5"`
	FileName    string `json:"fileName"`
	DownloadURL string `json:"downloadUrl"`
	FileSize    int64  `json:"fileSize"`
}

type PreviewInfoDTO struct {
	FileMD5   string `json:"fileMd5"`
	FileName  string `json:"fileName"`
	Content   string `json:"content"`
	FileSize  int64  `json:"fileSize"`
	Truncated bool   `json:"truncated"`
}

const (
	defaultDocumentDownloadExpiry = time.Hour
	defaultPreviewContentLimit    = 12000
)

type documentUserOrgTagProvider interface {
	GetUserEffectiveOrgTags(userID uint) ([]model.OrganizationTag, error)
}

type documentStorage interface {
	GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (io.ReadCloser, error)
	PresignedGetObject(ctx context.Context, bucketName, objectName string, expires time.Duration, reqParams url.Values) (*url.URL, error)
	RemoveObject(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error
}

type documentTextExtractor interface {
	ExtractText(ctx context.Context, reader io.Reader, fileName string) (string, error)
}

type documentESClient interface {
	DeleteDocumentsByFileMD5(ctx context.Context, fileMD5 string) error
}

type DocumentService interface {
	ListAccessibleFiles(ctx context.Context, user *model.User) ([]FileUploadDTO, error)
	ListUploadedFiles(ctx context.Context, userID uint) ([]FileUploadDTO, error)
	DeleteDocument(ctx context.Context, fileMD5 string, user *model.User, targetUserID *uint) error
	GenerateDownloadURL(ctx context.Context, fileMD5 string, fileName string, user *model.User) (*DownloadInfoDTO, error)
	GetFilePreviewContent(ctx context.Context, fileMD5 string, fileName string, user *model.User) (*PreviewInfoDTO, error)
}

type documentService struct {
	uploadRepo      repository.UploadRepository
	orgTagRepo      repository.OrganizationTagRepository
	userTagProvider documentUserOrgTagProvider
	minioClient     documentStorage
	bucketName      string
	tikaClient      documentTextExtractor
	docVectorRepo   repository.DocumentVectorRepository
	esClient        documentESClient
}

type minioDocumentStorage struct {
	client *minio.Client
}

func NewMinioDocumentStorage(client *minio.Client) documentStorage {
	return &minioDocumentStorage{client: client}
}

func (s *minioDocumentStorage) GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (io.ReadCloser, error) {
	if s.client == nil {
		return nil, ErrServiceUnavailable
	}
	return s.client.GetObject(ctx, bucketName, objectName, opts)
}

func (s *minioDocumentStorage) PresignedGetObject(ctx context.Context, bucketName, objectName string, expires time.Duration, reqParams url.Values) (*url.URL, error) {
	if s.client == nil {
		return nil, ErrServiceUnavailable
	}
	return s.client.PresignedGetObject(ctx, bucketName, objectName, expires, reqParams)
}

func (s *minioDocumentStorage) RemoveObject(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error {
	if s.client == nil {
		return ErrServiceUnavailable
	}
	return s.client.RemoveObject(ctx, bucketName, objectName, opts)
}

func NewDocumentService(
	uploadRepo repository.UploadRepository,
	orgTagRepo repository.OrganizationTagRepository,
	userTagProvider documentUserOrgTagProvider,
	minioClient documentStorage,
	bucketName string,
	tikaClient documentTextExtractor,
	docVectorRepo repository.DocumentVectorRepository,
	esClient documentESClient,
) DocumentService {
	return &documentService{
		uploadRepo:      uploadRepo,
		orgTagRepo:      orgTagRepo,
		userTagProvider: userTagProvider,
		minioClient:     minioClient,
		bucketName:      bucketName,
		tikaClient:      tikaClient,
		docVectorRepo:   docVectorRepo,
		esClient:        esClient,
	}
}

func (s *documentService) ListAccessibleFiles(ctx context.Context, user *model.User) ([]FileUploadDTO, error) {
	if s.uploadRepo == nil || s.orgTagRepo == nil || s.userTagProvider == nil {
		return nil, ErrServiceUnavailable
	}
	if user == nil {
		return nil, ErrInvalidInput
	}

	orgTags, err := s.userTagProvider.GetUserEffectiveOrgTags(user.ID)
	if err != nil {
		return nil, err
	}

	files, err := s.uploadRepo.FindAccessibleFiles(user.ID, extractOrgTagIDs(orgTags))
	if err != nil {
		log.Errorf("ListAccessibleFiles: query failed: %v", err)
		return nil, ErrInternal
	}
	return s.mapFileUploadsToDTOs(files)
}

func (s *documentService) ListUploadedFiles(ctx context.Context, userID uint) ([]FileUploadDTO, error) {
	if s.uploadRepo == nil || s.orgTagRepo == nil {
		return nil, ErrServiceUnavailable
	}
	if userID == 0 {
		return nil, ErrInvalidInput
	}

	files, err := s.uploadRepo.FindFilesByUserID(userID)
	if err != nil {
		log.Errorf("ListUploadedFiles: query failed: %v", err)
		return nil, ErrInternal
	}
	return s.mapFileUploadsToDTOs(files)
}

func (s *documentService) DeleteDocument(ctx context.Context, fileMD5 string, user *model.User, targetUserID *uint) error {
	if s.uploadRepo == nil || s.docVectorRepo == nil || s.esClient == nil || s.minioClient == nil {
		return ErrServiceUnavailable
	}
	if user == nil || strings.TrimSpace(fileMD5) == "" {
		return ErrInvalidInput
	}

	upload, err := s.resolveDeletionTarget(strings.TrimSpace(fileMD5), user, targetUserID)
	if err != nil {
		return err
	}

	if err := s.esClient.DeleteDocumentsByFileMD5(ctx, upload.FileMD5); err != nil {
		log.Errorf("DeleteDocument: delete elasticsearch docs failed: %v", err)
		return ErrInternal
	}
	if err := s.docVectorRepo.DeleteByFileMD5(upload.FileMD5); err != nil {
		log.Errorf("DeleteDocument: delete document vectors failed: %v", err)
		return ErrInternal
	}

	chunks, err := s.uploadRepo.FindChunksByFileMD5(upload.FileMD5)
	if err != nil {
		log.Errorf("DeleteDocument: list chunk infos failed: %v", err)
		return ErrInternal
	}

	if err := s.minioClient.RemoveObject(ctx, s.bucketName, buildUploadObjectKey(upload.UserID, upload.FileMD5, upload.FileName), minio.RemoveObjectOptions{}); err != nil {
		log.Errorf("DeleteDocument: remove merged object failed: %v", err)
		return ErrInternal
	}
	for _, chunk := range chunks {
		if err := s.minioClient.RemoveObject(ctx, s.bucketName, chunk.StoragePath, minio.RemoveObjectOptions{}); err != nil {
			log.Errorf("DeleteDocument: remove chunk object failed: path=%s err=%v", chunk.StoragePath, err)
			return ErrInternal
		}
	}

	if err := s.uploadRepo.DeleteUploadMark(ctx, upload.FileMD5, upload.UserID); err != nil {
		log.Errorf("DeleteDocument: delete upload mark failed: %v", err)
		return ErrInternal
	}
	if err := s.uploadRepo.DeleteChunkInfosByFileMD5(upload.FileMD5); err != nil {
		log.Errorf("DeleteDocument: delete chunk infos failed: %v", err)
		return ErrInternal
	}
	if err := s.uploadRepo.DeleteFileUploadRecord(upload.FileMD5, upload.UserID); err != nil {
		log.Errorf("DeleteDocument: delete upload record failed: %v", err)
		return ErrInternal
	}
	return nil
}

func (s *documentService) resolveDeletionTarget(fileMD5 string, user *model.User, targetUserID *uint) (*model.FileUpload, error) {
	lookup := func(ownerUserID uint) (*model.FileUpload, error) {
		upload, err := s.uploadRepo.FindByFileMD5AndUserID(fileMD5, ownerUserID)
		if err != nil {
			if strings.EqualFold(user.Role, "ADMIN") {
				log.Warnf("DeleteDocument: find upload failed: actor=%d target=%d md5=%s err=%v", user.ID, ownerUserID, fileMD5, err)
			} else {
				log.Warnf("DeleteDocument: find upload failed: user=%d md5=%s err=%v", user.ID, fileMD5, err)
			}
			return nil, ErrFileNotFound
		}
		return upload, nil
	}

	if !strings.EqualFold(user.Role, "ADMIN") {
		return lookup(user.ID)
	}

	if targetUserID != nil && *targetUserID != 0 {
		return lookup(*targetUserID)
	}

	uploads, err := s.uploadRepo.FindBatchByMD5s([]string{fileMD5})
	if err != nil {
		log.Errorf("DeleteDocument: admin batch lookup failed: md5=%s err=%v", fileMD5, err)
		return nil, ErrInternal
	}
	switch len(uploads) {
	case 0:
		return nil, ErrFileNotFound
	case 1:
		return &uploads[0], nil
	default:
		return nil, fmt.Errorf("%w: ambiguous file ownership, please specify userId", ErrInvalidInput)
	}
}

func (s *documentService) GenerateDownloadURL(ctx context.Context, fileMD5 string, fileName string, user *model.User) (*DownloadInfoDTO, error) {
	if s.uploadRepo == nil || s.userTagProvider == nil || s.minioClient == nil {
		return nil, ErrServiceUnavailable
	}
	upload, err := s.resolveAccessibleFile(ctx, fileMD5, fileName, user)
	if err != nil {
		return nil, err
	}

	link, err := s.minioClient.PresignedGetObject(ctx, s.bucketName, buildUploadObjectKey(upload.UserID, upload.FileMD5, upload.FileName), defaultDocumentDownloadExpiry, url.Values{})
	if err != nil {
		log.Errorf("GenerateDownloadURL: generate presigned url failed: %v", err)
		return nil, ErrInternal
	}

	return &DownloadInfoDTO{
		FileMD5:     upload.FileMD5,
		FileName:    upload.FileName,
		DownloadURL: link.String(),
		FileSize:    upload.TotalSize,
	}, nil
}

func (s *documentService) GetFilePreviewContent(ctx context.Context, fileMD5 string, fileName string, user *model.User) (*PreviewInfoDTO, error) {
	if s.uploadRepo == nil || s.userTagProvider == nil || s.minioClient == nil || s.tikaClient == nil {
		return nil, ErrServiceUnavailable
	}
	upload, err := s.resolveAccessibleFile(ctx, fileMD5, fileName, user)
	if err != nil {
		return nil, err
	}

	object, err := s.minioClient.GetObject(ctx, s.bucketName, buildUploadObjectKey(upload.UserID, upload.FileMD5, upload.FileName), minio.GetObjectOptions{})
	if err != nil {
		log.Errorf("GetFilePreviewContent: get object failed: %v", err)
		return nil, ErrInternal
	}
	defer object.Close()

	content, err := s.tikaClient.ExtractText(ctx, object, upload.FileName)
	if err != nil {
		log.Errorf("GetFilePreviewContent: tika extract failed: %v", err)
		return nil, ErrInternal
	}

	truncated := false
	if len([]rune(content)) > defaultPreviewContentLimit {
		content = string([]rune(content)[:defaultPreviewContentLimit])
		truncated = true
	}

	return &PreviewInfoDTO{
		FileMD5:   upload.FileMD5,
		FileName:  upload.FileName,
		Content:   content,
		FileSize:  upload.TotalSize,
		Truncated: truncated,
	}, nil
}

func (s *documentService) resolveAccessibleFile(ctx context.Context, fileMD5 string, fileName string, user *model.User) (*model.FileUpload, error) {
	if user == nil {
		return nil, ErrInvalidInput
	}

	orgTags, err := s.userTagProvider.GetUserEffectiveOrgTags(user.ID)
	if err != nil {
		return nil, err
	}
	tagIDs := extractOrgTagIDs(orgTags)

	if strings.TrimSpace(fileMD5) != "" {
		upload, err := s.uploadRepo.FindAccessibleFileByMD5(user.ID, tagIDs, strings.TrimSpace(fileMD5))
		if err != nil {
			log.Warnf("resolveAccessibleFile: find by md5 failed: user=%d md5=%s err=%v", user.ID, fileMD5, err)
			return nil, ErrFileNotFound
		}
		return upload, nil
	}

	trimmedName := strings.TrimSpace(fileName)
	if trimmedName == "" {
		return nil, ErrInvalidInput
	}

	uploads, err := s.uploadRepo.FindAccessibleFilesByName(user.ID, tagIDs, trimmedName)
	if err != nil {
		log.Errorf("resolveAccessibleFile: find by name failed: %v", err)
		return nil, ErrInternal
	}
	switch len(uploads) {
	case 0:
		return nil, ErrFileNotFound
	case 1:
		return &uploads[0], nil
	default:
		return nil, fmt.Errorf("%w: fileName is ambiguous, please use fileMd5", ErrInvalidInput)
	}
}

func (s *documentService) mapFileUploadsToDTOs(files []model.FileUpload) ([]FileUploadDTO, error) {
	if len(files) == 0 {
		return []FileUploadDTO{}, nil
	}

	tagIDs := make([]string, 0, len(files))
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		tagID := strings.TrimSpace(file.OrgTag)
		if tagID == "" {
			continue
		}
		if _, exists := seen[tagID]; exists {
			continue
		}
		seen[tagID] = struct{}{}
		tagIDs = append(tagIDs, tagID)
	}

	tags, err := s.orgTagRepo.FindBatchByIDs(tagIDs)
	if err != nil {
		log.Errorf("mapFileUploadsToDTOs: query org tags failed: %v", err)
		return nil, ErrInternal
	}

	tagNameByID := make(map[string]string, len(tags))
	for _, tag := range tags {
		tagNameByID[tag.TagID] = tag.Name
	}

	result := make([]FileUploadDTO, 0, len(files))
	for _, file := range files {
		result = append(result, FileUploadDTO{
			FileUpload: file,
			OrgTagName: tagNameByID[file.OrgTag],
		})
	}
	return result, nil
}
