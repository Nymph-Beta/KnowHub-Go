package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/internal/service"

	"github.com/gin-gonic/gin"
)

type fakeSearchService struct {
	hybridSearchFn func(ctx context.Context, query string, topK int, user *model.User) ([]model.SearchResponseDTO, error)
}

func (f *fakeSearchService) HybridSearch(ctx context.Context, query string, topK int, user *model.User) ([]model.SearchResponseDTO, error) {
	if f.hybridSearchFn != nil {
		return f.hybridSearchFn(ctx, query, topK, user)
	}
	return nil, nil
}

func newSearchRouter(h *SearchHandler) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &model.User{ID: 7, Username: "alice"})
		c.Next()
	})
	r.GET("/search/hybrid", h.HybridSearch)
	return r
}

func TestSearchHandler_HybridSearch_Success(t *testing.T) {
	r := newSearchRouter(NewSearchHandler(&fakeSearchService{
		hybridSearchFn: func(ctx context.Context, query string, topK int, user *model.User) ([]model.SearchResponseDTO, error) {
			if query != "go 并发" {
				t.Fatalf("unexpected query: %s", query)
			}
			if topK != 5 {
				t.Fatalf("unexpected topK: %d", topK)
			}
			if user == nil || user.ID != 7 {
				t.Fatalf("unexpected user: %+v", user)
			}
			return []model.SearchResponseDTO{{
				FileMD5:     "md5",
				FileName:    "go.pdf",
				ChunkID:     3,
				TextContent: "Go 的并发模型基于 goroutine",
				Score:       8.2,
			}}, nil
		},
	}))

	req := httptest.NewRequest(http.MethodGet, "/search/hybrid?query=go%20%E5%B9%B6%E5%8F%91&topK=5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp["message"] != "Search successful" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestSearchHandler_HybridSearch_InvalidTopK(t *testing.T) {
	r := newSearchRouter(NewSearchHandler(&fakeSearchService{}))

	req := httptest.NewRequest(http.MethodGet, "/search/hybrid?query=go&topK=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
}

func TestSearchHandler_HybridSearch_ServiceError(t *testing.T) {
	r := newSearchRouter(NewSearchHandler(&fakeSearchService{
		hybridSearchFn: func(ctx context.Context, query string, topK int, user *model.User) ([]model.SearchResponseDTO, error) {
			return nil, service.ErrInvalidInput
		},
	}))

	req := httptest.NewRequest(http.MethodGet, "/search/hybrid?query=", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
}

func TestSearchHandler_HybridSearch_Unavailable(t *testing.T) {
	r := newSearchRouter(NewSearchHandler(nil))

	req := httptest.NewRequest(http.MethodGet, "/search/hybrid?query=go", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
}

func TestSearchHandler_HybridSearch_InternalError(t *testing.T) {
	r := newSearchRouter(NewSearchHandler(&fakeSearchService{
		hybridSearchFn: func(ctx context.Context, query string, topK int, user *model.User) ([]model.SearchResponseDTO, error) {
			return nil, errors.New("boom")
		},
	}))

	req := httptest.NewRequest(http.MethodGet, "/search/hybrid?query=go", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
}
