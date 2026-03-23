package service

import (
	"context"
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	"pai_smart_go_v2/internal/model"

	"github.com/minio/minio-go/v7"
	"gorm.io/gorm"
)

type fakeDocumentUserTagProvider struct {
	getUserEffectiveOrgTagsFn func(userID uint) ([]model.OrganizationTag, error)
}

func (f *fakeDocumentUserTagProvider) GetUserEffectiveOrgTags(userID uint) ([]model.OrganizationTag, error) {
	if f.getUserEffectiveOrgTagsFn != nil {
		return f.getUserEffectiveOrgTagsFn(userID)
	}
	return []model.OrganizationTag{}, nil
}

type fakeDocumentStorage struct {
	getObjectFn          func(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (io.ReadCloser, error)
	presignedGetObjectFn func(ctx context.Context, bucketName, objectName string, expires time.Duration, reqParams url.Values) (*url.URL, error)
	removeObjectFn       func(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error
}

func (f *fakeDocumentStorage) GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (io.ReadCloser, error) {
	if f.getObjectFn != nil {
		return f.getObjectFn(ctx, bucketName, objectName, opts)
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func (f *fakeDocumentStorage) PresignedGetObject(ctx context.Context, bucketName, objectName string, expires time.Duration, reqParams url.Values) (*url.URL, error) {
	if f.presignedGetObjectFn != nil {
		return f.presignedGetObjectFn(ctx, bucketName, objectName, expires, reqParams)
	}
	return url.Parse("https://example.com/download")
}

func (f *fakeDocumentStorage) RemoveObject(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error {
	if f.removeObjectFn != nil {
		return f.removeObjectFn(ctx, bucketName, objectName, opts)
	}
	return nil
}

type fakeDocumentTextExtractor struct {
	extractTextFn func(ctx context.Context, reader io.Reader, fileName string) (string, error)
}

func (f *fakeDocumentTextExtractor) ExtractText(ctx context.Context, reader io.Reader, fileName string) (string, error) {
	if f.extractTextFn != nil {
		return f.extractTextFn(ctx, reader, fileName)
	}
	return "", nil
}

type fakeDocumentVectorRepo struct {
	deleteByFileMD5Fn func(fileMD5 string) error
}

func (f *fakeDocumentVectorRepo) BatchCreate(vectors []model.DocumentVector) error { return nil }
func (f *fakeDocumentVectorRepo) FindByFileMD5(fileMD5 string) ([]model.DocumentVector, error) {
	return []model.DocumentVector{}, nil
}
func (f *fakeDocumentVectorRepo) DeleteByFileMD5(fileMD5 string) error {
	if f.deleteByFileMD5Fn != nil {
		return f.deleteByFileMD5Fn(fileMD5)
	}
	return nil
}

type fakeDocumentESClient struct {
	deleteDocumentsByFileMD5Fn func(ctx context.Context, fileMD5 string) error
}

func (f *fakeDocumentESClient) DeleteDocumentsByFileMD5(ctx context.Context, fileMD5 string) error {
	if f.deleteDocumentsByFileMD5Fn != nil {
		return f.deleteDocumentsByFileMD5Fn(ctx, fileMD5)
	}
	return nil
}

func TestDocumentService_ListAccessibleFiles(t *testing.T) {
	svc := NewDocumentService(
		&fakeUploadRepo{
			findAccessibleFilesFn: func(userID uint, orgTags []string) ([]model.FileUpload, error) {
				if userID != 7 || len(orgTags) != 2 || orgTags[0] != "team-a" || orgTags[1] != "team-b" {
					t.Fatalf("unexpected query args: user=%d tags=%v", userID, orgTags)
				}
				return []model.FileUpload{{FileMD5: "md5-a", FileName: "a.pdf", OrgTag: "team-a"}}, nil
			},
		},
		&fakeOrgTagRepo{
			findBatchByIDsFn: func(tagIDs []string) ([]model.OrganizationTag, error) {
				return []model.OrganizationTag{{TagID: "team-a", Name: "Team A"}}, nil
			},
		},
		&fakeDocumentUserTagProvider{
			getUserEffectiveOrgTagsFn: func(userID uint) ([]model.OrganizationTag, error) {
				return []model.OrganizationTag{{TagID: "team-a"}, {TagID: "team-b"}}, nil
			},
		},
		&fakeDocumentStorage{},
		"bucket-a",
		&fakeDocumentTextExtractor{},
		&fakeDocumentVectorRepo{},
		&fakeDocumentESClient{},
	)

	files, err := svc.ListAccessibleFiles(context.Background(), &model.User{ID: 7})
	if err != nil {
		t.Fatalf("ListAccessibleFiles() error = %v", err)
	}
	if len(files) != 1 || files[0].OrgTagName != "Team A" {
		t.Fatalf("unexpected files: %+v", files)
	}
}

func TestDocumentService_GenerateDownloadURL_AmbiguousFileName(t *testing.T) {
	svc := NewDocumentService(
		&fakeUploadRepo{
			findAccessibleFilesByNameFn: func(userID uint, orgTags []string, fileName string) ([]model.FileUpload, error) {
				return []model.FileUpload{
					{FileMD5: "a", FileName: fileName, UserID: userID},
					{FileMD5: "b", FileName: fileName, UserID: userID},
				}, nil
			},
		},
		&fakeOrgTagRepo{},
		&fakeDocumentUserTagProvider{},
		&fakeDocumentStorage{},
		"bucket-a",
		&fakeDocumentTextExtractor{},
		&fakeDocumentVectorRepo{},
		&fakeDocumentESClient{},
	)

	_, err := svc.GenerateDownloadURL(context.Background(), "", "dup.pdf", &model.User{ID: 3})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestDocumentService_DeleteDocument_CompleteCleanup(t *testing.T) {
	callOrder := make([]string, 0)
	svc := NewDocumentService(
		&fakeUploadRepo{
			findByFileMD5AndUserIDFn: func(fileMD5 string, userID uint) (*model.FileUpload, error) {
				return &model.FileUpload{FileMD5: fileMD5, FileName: "doc.pdf", UserID: userID}, nil
			},
			findChunksByFileMD5Fn: func(fileMD5 string) ([]model.ChunkInfo, error) {
				return []model.ChunkInfo{
					{StoragePath: "chunks/md5v/0"},
					{StoragePath: "chunks/md5v/1"},
				}, nil
			},
			deleteUploadMarkFn: func(ctx context.Context, fileMD5 string, userID uint) error {
				callOrder = append(callOrder, "delete-upload-mark")
				return nil
			},
			deleteChunkInfosByFileMD5Fn: func(fileMD5 string) error {
				callOrder = append(callOrder, "delete-chunk-info")
				return nil
			},
			deleteFileUploadRecordFn: func(fileMD5 string, userID uint) error {
				callOrder = append(callOrder, "delete-upload-record")
				return nil
			},
		},
		&fakeOrgTagRepo{},
		&fakeDocumentUserTagProvider{},
		&fakeDocumentStorage{
			removeObjectFn: func(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error {
				callOrder = append(callOrder, "remove:"+objectName)
				return nil
			},
		},
		"bucket-a",
		&fakeDocumentTextExtractor{},
		&fakeDocumentVectorRepo{
			deleteByFileMD5Fn: func(fileMD5 string) error {
				callOrder = append(callOrder, "delete-vectors")
				return nil
			},
		},
		&fakeDocumentESClient{
			deleteDocumentsByFileMD5Fn: func(ctx context.Context, fileMD5 string) error {
				callOrder = append(callOrder, "delete-es")
				return nil
			},
		},
	)

	err := svc.DeleteDocument(context.Background(), "md5v", &model.User{ID: 5})
	if err != nil {
		t.Fatalf("DeleteDocument() error = %v", err)
	}

	expected := []string{
		"delete-es",
		"delete-vectors",
		"remove:uploads/5/md5v/doc.pdf",
		"remove:chunks/md5v/0",
		"remove:chunks/md5v/1",
		"delete-upload-mark",
		"delete-chunk-info",
		"delete-upload-record",
	}
	if strings.Join(callOrder, ",") != strings.Join(expected, ",") {
		t.Fatalf("unexpected cleanup order: got=%v want=%v", callOrder, expected)
	}
}

func TestDocumentService_GetFilePreviewContent(t *testing.T) {
	svc := NewDocumentService(
		&fakeUploadRepo{
			findAccessibleFileByMD5Fn: func(userID uint, orgTags []string, fileMD5 string) (*model.FileUpload, error) {
				return &model.FileUpload{FileMD5: fileMD5, FileName: "doc.pdf", UserID: 9, TotalSize: 128}, nil
			},
		},
		&fakeOrgTagRepo{},
		&fakeDocumentUserTagProvider{},
		&fakeDocumentStorage{
			getObjectFn: func(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader("pdf-bytes")), nil
			},
		},
		"bucket-a",
		&fakeDocumentTextExtractor{
			extractTextFn: func(ctx context.Context, reader io.Reader, fileName string) (string, error) {
				payload, _ := io.ReadAll(reader)
				if string(payload) != "pdf-bytes" || fileName != "doc.pdf" {
					t.Fatalf("unexpected extractor input: %q %s", string(payload), fileName)
				}
				return "preview content", nil
			},
		},
		&fakeDocumentVectorRepo{},
		&fakeDocumentESClient{},
	)

	info, err := svc.GetFilePreviewContent(context.Background(), "md5v", "", &model.User{ID: 9})
	if err != nil {
		t.Fatalf("GetFilePreviewContent() error = %v", err)
	}
	if info.Content != "preview content" || info.FileSize != 128 {
		t.Fatalf("unexpected preview info: %+v", info)
	}
}

func TestDocumentService_DeleteDocument_NotFound(t *testing.T) {
	svc := NewDocumentService(
		&fakeUploadRepo{
			findByFileMD5AndUserIDFn: func(fileMD5 string, userID uint) (*model.FileUpload, error) {
				return nil, gorm.ErrRecordNotFound
			},
		},
		&fakeOrgTagRepo{},
		&fakeDocumentUserTagProvider{},
		&fakeDocumentStorage{},
		"bucket-a",
		&fakeDocumentTextExtractor{},
		&fakeDocumentVectorRepo{},
		&fakeDocumentESClient{},
	)

	err := svc.DeleteDocument(context.Background(), "missing", &model.User{ID: 1})
	if !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("expected ErrFileNotFound, got %v", err)
	}
}
