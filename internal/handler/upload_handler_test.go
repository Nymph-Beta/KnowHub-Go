package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/internal/service"

	"github.com/gin-gonic/gin"
)

type fakeUploadServiceForHandler struct {
	simpleUploadFn    func(ctx context.Context, userID uint, orgTag, fileName string, fileSize int64, reader io.Reader) (*service.UploadResult, error)
	downloadFileFn    func(ctx context.Context, fileMD5 string, userID uint) (*service.DownloadResult, error)
	checkFileFn       func(ctx context.Context, fileMD5 string, userID uint) (*service.CheckResult, error)
	getStatusFn       func(ctx context.Context, fileMD5 string, userID uint) (*service.UploadStatusResult, error)
	fastUploadFn      func(ctx context.Context, fileMD5 string, userID uint) (*service.FastUploadCheckResult, error)
	getSupportedTypes func() []string
	uploadChunkFn     func(ctx context.Context, fileMD5 string, fileName string, totalSize int64, chunkIndex int, reader io.Reader, chunkSize int64, userID uint, orgTag string, isPublic bool) (*service.ChunkUploadResult, error)
	mergeChunksFn     func(ctx context.Context, fileMD5 string, fileName string, userID uint) (*service.MergeResult, error)
}

func (f *fakeUploadServiceForHandler) SimpleUpload(ctx context.Context, userID uint, orgTag, fileName string, fileSize int64, reader io.Reader) (*service.UploadResult, error) {
	if f.simpleUploadFn != nil {
		return f.simpleUploadFn(ctx, userID, orgTag, fileName, fileSize, reader)
	}
	return nil, nil
}

func (f *fakeUploadServiceForHandler) DownloadFile(ctx context.Context, fileMD5 string, userID uint) (*service.DownloadResult, error) {
	if f.downloadFileFn != nil {
		return f.downloadFileFn(ctx, fileMD5, userID)
	}
	return nil, nil
}

func (f *fakeUploadServiceForHandler) CheckFile(ctx context.Context, fileMD5 string, userID uint) (*service.CheckResult, error) {
	if f.checkFileFn != nil {
		return f.checkFileFn(ctx, fileMD5, userID)
	}
	return &service.CheckResult{Completed: false, UploadedChunks: []int{}}, nil
}

func (f *fakeUploadServiceForHandler) GetUploadStatus(ctx context.Context, fileMD5 string, userID uint) (*service.UploadStatusResult, error) {
	if f.getStatusFn != nil {
		return f.getStatusFn(ctx, fileMD5, userID)
	}
	return &service.UploadStatusResult{FileMD5: fileMD5, Status: 0, Completed: false, UploadedChunks: []int{}, Progress: 0}, nil
}

func (f *fakeUploadServiceForHandler) CheckFastUpload(ctx context.Context, fileMD5 string, userID uint) (*service.FastUploadCheckResult, error) {
	if f.fastUploadFn != nil {
		return f.fastUploadFn(ctx, fileMD5, userID)
	}
	return &service.FastUploadCheckResult{CanQuickUpload: false}, nil
}

func (f *fakeUploadServiceForHandler) GetSupportedTypes() []string {
	if f.getSupportedTypes != nil {
		return f.getSupportedTypes()
	}
	return []string{".pdf"}
}

func (f *fakeUploadServiceForHandler) UploadChunk(ctx context.Context, fileMD5 string, fileName string, totalSize int64, chunkIndex int, reader io.Reader, chunkSize int64, userID uint, orgTag string, isPublic bool) (*service.ChunkUploadResult, error) {
	if f.uploadChunkFn != nil {
		return f.uploadChunkFn(ctx, fileMD5, fileName, totalSize, chunkIndex, reader, chunkSize, userID, orgTag, isPublic)
	}
	return &service.ChunkUploadResult{UploadedChunks: []int{}, Progress: 0}, nil
}

func (f *fakeUploadServiceForHandler) MergeChunks(ctx context.Context, fileMD5 string, fileName string, userID uint) (*service.MergeResult, error) {
	if f.mergeChunksFn != nil {
		return f.mergeChunksFn(ctx, fileMD5, fileName, userID)
	}
	return &service.MergeResult{ObjectURL: "", FileMD5: fileMD5, FileName: fileName}, nil
}

func newUploadPhase7Router(h *UploadHandler) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &model.User{ID: 99, Username: "tester"})
		c.Next()
	})
	r.POST("/upload/check", h.CheckFile)
	r.GET("/upload/status", h.GetUploadStatus)
	r.GET("/upload/supported-types", h.GetSupportedTypes)
	r.POST("/upload/fast-upload", h.FastUpload)
	r.POST("/upload/chunk", h.UploadChunk)
	r.POST("/upload/merge", h.MergeChunks)
	return r
}

func doMultipartReq(r http.Handler, path string, fields map[string]string, fileField, fileName string, fileBytes []byte) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for k, v := range fields {
		_ = writer.WriteField(k, v)
	}
	if fileField != "" {
		part, _ := writer.CreateFormFile(fileField, fileName)
		if len(fileBytes) > 0 {
			_, _ = part.Write(fileBytes)
		}
	}
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestUploadHandler_CheckFile_InvalidBody(t *testing.T) {
	svc := &fakeUploadServiceForHandler{}
	r := newUploadPhase7Router(NewUploadHandler(svc))

	w := doReq(r, http.MethodPost, "/upload/check", `{}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expect 400, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestUploadHandler_CheckFile_Success(t *testing.T) {
	var gotMD5 string
	var gotUserID uint
	svc := &fakeUploadServiceForHandler{
		checkFileFn: func(ctx context.Context, fileMD5 string, userID uint) (*service.CheckResult, error) {
			gotMD5 = fileMD5
			gotUserID = userID
			return &service.CheckResult{Completed: false, UploadedChunks: []int{0, 2}}, nil
		},
	}
	r := newUploadPhase7Router(NewUploadHandler(svc))

	w := doReq(r, http.MethodPost, "/upload/check", `{"md5":"abc123"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expect 200, got %d, body=%s", w.Code, w.Body.String())
	}
	if gotMD5 != "abc123" || gotUserID != 99 {
		t.Fatalf("unexpected service args: md5=%s userID=%d", gotMD5, gotUserID)
	}
}

func TestUploadHandler_GetUploadStatus_Success(t *testing.T) {
	var gotMD5 string
	svc := &fakeUploadServiceForHandler{
		getStatusFn: func(ctx context.Context, fileMD5 string, userID uint) (*service.UploadStatusResult, error) {
			gotMD5 = fileMD5
			if userID != 99 {
				t.Fatalf("unexpected user id: %d", userID)
			}
			return &service.UploadStatusResult{
				FileMD5:        fileMD5,
				Status:         0,
				Completed:      false,
				UploadedChunks: []int{0, 1},
				Progress:       50,
			}, nil
		},
	}
	r := newUploadPhase7Router(NewUploadHandler(svc))

	req := httptest.NewRequest(http.MethodGet, "/upload/status?fileMd5=abc123", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expect 200, got %d, body=%s", w.Code, w.Body.String())
	}
	if gotMD5 != "abc123" {
		t.Fatalf("unexpected md5: %s", gotMD5)
	}
}

func TestUploadHandler_GetSupportedTypes_Success(t *testing.T) {
	svc := &fakeUploadServiceForHandler{
		getSupportedTypes: func() []string {
			return []string{".docx", ".pdf"}
		},
	}
	r := newUploadPhase7Router(NewUploadHandler(svc))

	req := httptest.NewRequest(http.MethodGet, "/upload/supported-types", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expect 200, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestUploadHandler_FastUpload_Success(t *testing.T) {
	var gotMD5 string
	svc := &fakeUploadServiceForHandler{
		fastUploadFn: func(ctx context.Context, fileMD5 string, userID uint) (*service.FastUploadCheckResult, error) {
			gotMD5 = fileMD5
			if userID != 99 {
				t.Fatalf("unexpected user id: %d", userID)
			}
			return &service.FastUploadCheckResult{CanQuickUpload: true, FileMD5: fileMD5, FileName: "a.pdf"}, nil
		},
	}
	r := newUploadPhase7Router(NewUploadHandler(svc))

	w := doReq(r, http.MethodPost, "/upload/fast-upload", `{"md5":"abc123"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expect 200, got %d, body=%s", w.Code, w.Body.String())
	}
	if gotMD5 != "abc123" {
		t.Fatalf("unexpected md5: %s", gotMD5)
	}
}

func TestUploadHandler_UploadChunk_MissingRequiredFields(t *testing.T) {
	svc := &fakeUploadServiceForHandler{}
	r := newUploadPhase7Router(NewUploadHandler(svc))

	w := doMultipartReq(r, "/upload/chunk", map[string]string{
		"fileName": "a.pdf",
	}, "file", "chunk.bin", []byte("part"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expect 400, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestUploadHandler_UploadChunk_InvalidTotalSize(t *testing.T) {
	svc := &fakeUploadServiceForHandler{}
	r := newUploadPhase7Router(NewUploadHandler(svc))

	w := doMultipartReq(r, "/upload/chunk", map[string]string{
		"fileMd5":    "md5-x",
		"fileName":   "a.pdf",
		"totalSize":  "not-int",
		"chunkIndex": "0",
	}, "file", "chunk.bin", []byte("part"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expect 400, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestUploadHandler_UploadChunk_InvalidChunkIndex(t *testing.T) {
	svc := &fakeUploadServiceForHandler{}
	r := newUploadPhase7Router(NewUploadHandler(svc))

	w := doMultipartReq(r, "/upload/chunk", map[string]string{
		"fileMd5":    "md5-x",
		"fileName":   "a.pdf",
		"totalSize":  "10",
		"chunkIndex": "x",
	}, "file", "chunk.bin", []byte("part"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expect 400, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestUploadHandler_UploadChunk_MissingFile(t *testing.T) {
	svc := &fakeUploadServiceForHandler{}
	r := newUploadPhase7Router(NewUploadHandler(svc))

	w := doMultipartReq(r, "/upload/chunk", map[string]string{
		"fileMd5":    "md5-x",
		"fileName":   "a.pdf",
		"totalSize":  "10",
		"chunkIndex": "0",
	}, "", "", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expect 400, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestUploadHandler_UploadChunk_Success(t *testing.T) {
	var got struct {
		fileMD5    string
		fileName   string
		totalSize  int64
		chunkIndex int
		userID     uint
		orgTag     string
		isPublic   bool
		chunkSize  int64
	}

	svc := &fakeUploadServiceForHandler{
		uploadChunkFn: func(ctx context.Context, fileMD5 string, fileName string, totalSize int64, chunkIndex int, reader io.Reader, chunkSize int64, userID uint, orgTag string, isPublic bool) (*service.ChunkUploadResult, error) {
			got.fileMD5 = fileMD5
			got.fileName = fileName
			got.totalSize = totalSize
			got.chunkIndex = chunkIndex
			got.userID = userID
			got.orgTag = orgTag
			got.isPublic = isPublic
			got.chunkSize = chunkSize
			return &service.ChunkUploadResult{UploadedChunks: []int{0, 1}, Progress: 66.6}, nil
		},
	}
	r := newUploadPhase7Router(NewUploadHandler(svc))

	w := doMultipartReq(r, "/upload/chunk", map[string]string{
		"fileMd5":    "md5-ok",
		"fileName":   "a.pdf",
		"totalSize":  "123",
		"chunkIndex": "1",
		"orgTag":     "team-x",
		"isPublic":   "1",
	}, "file", "chunk.bin", []byte("chunk-data"))
	if w.Code != http.StatusOK {
		t.Fatalf("expect 200, got %d, body=%s", w.Code, w.Body.String())
	}

	if got.fileMD5 != "md5-ok" || got.fileName != "a.pdf" || got.totalSize != 123 || got.chunkIndex != 1 {
		t.Fatalf("unexpected parsed fields: %+v", got)
	}
	if got.userID != 99 || got.orgTag != "team-x" || !got.isPublic {
		t.Fatalf("unexpected auth/org/isPublic parse: %+v", got)
	}
	if got.chunkSize <= 0 {
		t.Fatalf("unexpected chunkSize: %d", got.chunkSize)
	}
}

func TestUploadHandler_MergeChunks_InvalidBody(t *testing.T) {
	svc := &fakeUploadServiceForHandler{}
	r := newUploadPhase7Router(NewUploadHandler(svc))

	w := doReq(r, http.MethodPost, "/upload/merge", `{"fileMd5":"abc"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expect 400, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestUploadHandler_MergeChunks_ErrorMapping(t *testing.T) {
	svc := &fakeUploadServiceForHandler{
		mergeChunksFn: func(ctx context.Context, fileMD5 string, fileName string, userID uint) (*service.MergeResult, error) {
			return nil, service.ErrChunksIncomplete
		},
	}
	r := newUploadPhase7Router(NewUploadHandler(svc))

	w := doReq(r, http.MethodPost, "/upload/merge", `{"fileMd5":"abc","fileName":"a.pdf"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expect 400, got %d, body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json unmarshal error: %v", err)
	}
	if resp["message"] != "Not all chunks have been uploaded" {
		t.Fatalf("unexpected message: %v", resp["message"])
	}
}

func TestUploadHandler_MergeChunks_Success(t *testing.T) {
	var gotMD5, gotFileName string
	var gotUserID uint
	svc := &fakeUploadServiceForHandler{
		mergeChunksFn: func(ctx context.Context, fileMD5 string, fileName string, userID uint) (*service.MergeResult, error) {
			gotMD5 = fileMD5
			gotFileName = fileName
			gotUserID = userID
			return &service.MergeResult{
				ObjectURL: "uploads/99/md5-ok/a.pdf",
				FileMD5:   fileMD5,
				FileName:  fileName,
			}, nil
		},
	}
	r := newUploadPhase7Router(NewUploadHandler(svc))

	w := doReq(r, http.MethodPost, "/upload/merge", `{"fileMd5":"md5-ok","fileName":"a.pdf"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expect 200, got %d, body=%s", w.Code, w.Body.String())
	}
	if gotMD5 != "md5-ok" || gotFileName != "a.pdf" || gotUserID != 99 {
		t.Fatalf("unexpected service args: md5=%s file=%s user=%d", gotMD5, gotFileName, gotUserID)
	}
}
