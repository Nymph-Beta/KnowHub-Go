package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/internal/service"

	"github.com/gin-gonic/gin"
)

type fakeDocumentServiceForHandler struct {
	listAccessibleFilesFn  func(ctx context.Context, user *model.User) ([]service.FileUploadDTO, error)
	listUploadedFilesFn    func(ctx context.Context, userID uint) ([]service.FileUploadDTO, error)
	deleteDocumentFn       func(ctx context.Context, fileMD5 string, user *model.User) error
	generateDownloadURLFn  func(ctx context.Context, fileMD5 string, fileName string, user *model.User) (*service.DownloadInfoDTO, error)
	getFilePreviewContentFn func(ctx context.Context, fileMD5 string, fileName string, user *model.User) (*service.PreviewInfoDTO, error)
}

func (f *fakeDocumentServiceForHandler) ListAccessibleFiles(ctx context.Context, user *model.User) ([]service.FileUploadDTO, error) {
	if f.listAccessibleFilesFn != nil {
		return f.listAccessibleFilesFn(ctx, user)
	}
	return []service.FileUploadDTO{}, nil
}

func (f *fakeDocumentServiceForHandler) ListUploadedFiles(ctx context.Context, userID uint) ([]service.FileUploadDTO, error) {
	if f.listUploadedFilesFn != nil {
		return f.listUploadedFilesFn(ctx, userID)
	}
	return []service.FileUploadDTO{}, nil
}

func (f *fakeDocumentServiceForHandler) DeleteDocument(ctx context.Context, fileMD5 string, user *model.User) error {
	if f.deleteDocumentFn != nil {
		return f.deleteDocumentFn(ctx, fileMD5, user)
	}
	return nil
}

func (f *fakeDocumentServiceForHandler) GenerateDownloadURL(ctx context.Context, fileMD5 string, fileName string, user *model.User) (*service.DownloadInfoDTO, error) {
	if f.generateDownloadURLFn != nil {
		return f.generateDownloadURLFn(ctx, fileMD5, fileName, user)
	}
	return &service.DownloadInfoDTO{}, nil
}

func (f *fakeDocumentServiceForHandler) GetFilePreviewContent(ctx context.Context, fileMD5 string, fileName string, user *model.User) (*service.PreviewInfoDTO, error) {
	if f.getFilePreviewContentFn != nil {
		return f.getFilePreviewContentFn(ctx, fileMD5, fileName, user)
	}
	return &service.PreviewInfoDTO{}, nil
}

func newDocumentRouter(h *DocumentHandler) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &model.User{ID: 9, Username: "tester"})
		c.Next()
	})
	r.GET("/documents/accessible", h.ListAccessibleFiles)
	r.GET("/documents/uploads", h.ListUploadedFiles)
	r.DELETE("/documents/:fileMd5", h.DeleteDocument)
	r.GET("/documents/download", h.GenerateDownloadURL)
	r.GET("/documents/preview", h.PreviewFile)
	return r
}

func TestDocumentHandler_ListAccessibleFiles_Success(t *testing.T) {
	r := newDocumentRouter(NewDocumentHandler(&fakeDocumentServiceForHandler{
		listAccessibleFilesFn: func(ctx context.Context, user *model.User) ([]service.FileUploadDTO, error) {
			return []service.FileUploadDTO{{FileUpload: model.FileUpload{FileMD5: "md5"}}}, nil
		},
	}))

	w := doReq(r, http.MethodGet, "/documents/accessible", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expect 200, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestDocumentHandler_GenerateDownloadURL_MissingQuery(t *testing.T) {
	r := newDocumentRouter(NewDocumentHandler(&fakeDocumentServiceForHandler{}))

	req := httptest.NewRequest(http.MethodGet, "/documents/download", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expect 400, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestDocumentHandler_DeleteDocument_ErrorMapping(t *testing.T) {
	r := newDocumentRouter(NewDocumentHandler(&fakeDocumentServiceForHandler{
		deleteDocumentFn: func(ctx context.Context, fileMD5 string, user *model.User) error {
			return service.ErrFileNotFound
		},
	}))

	req := httptest.NewRequest(http.MethodDelete, "/documents/missing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expect 404, got %d, body=%s", w.Code, w.Body.String())
	}
}
