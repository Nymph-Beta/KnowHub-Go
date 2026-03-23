package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/internal/service"

	"github.com/gin-gonic/gin"
)

type fakeConversationServiceForHandler struct {
	getConversationHistoryFn func(ctx context.Context, userID uint) ([]model.ChatMessage, error)
	getAllConversationsFn    func(ctx context.Context, filter service.ConversationAdminFilter) ([]service.ConversationAdminRecord, error)
}

func (f *fakeConversationServiceForHandler) GetConversationHistory(ctx context.Context, userID uint) ([]model.ChatMessage, error) {
	if f.getConversationHistoryFn != nil {
		return f.getConversationHistoryFn(ctx, userID)
	}
	return []model.ChatMessage{}, nil
}

func (f *fakeConversationServiceForHandler) GetAllConversations(ctx context.Context, filter service.ConversationAdminFilter) ([]service.ConversationAdminRecord, error) {
	if f.getAllConversationsFn != nil {
		return f.getAllConversationsFn(ctx, filter)
	}
	return []service.ConversationAdminRecord{}, nil
}

func newConversationRouter(h *ConversationHandler) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &model.User{ID: 11, Username: "tester"})
		c.Next()
	})
	r.GET("/users/conversation", h.GetConversations)
	r.GET("/admin/conversation", h.GetAllConversations)
	return r
}

func TestConversationHandler_GetConversations_Success(t *testing.T) {
	r := newConversationRouter(NewConversationHandler(&fakeConversationServiceForHandler{
		getConversationHistoryFn: func(ctx context.Context, userID uint) ([]model.ChatMessage, error) {
			return []model.ChatMessage{{Role: "user", Content: "hello"}}, nil
		},
	}))

	req := httptest.NewRequest(http.MethodGet, "/users/conversation", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expect 200, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestConversationHandler_GetAllConversations_InvalidUserID(t *testing.T) {
	r := newConversationRouter(NewConversationHandler(&fakeConversationServiceForHandler{}))

	req := httptest.NewRequest(http.MethodGet, "/admin/conversation?userid=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expect 400, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestConversationHandler_GetAllConversations_Success(t *testing.T) {
	r := newConversationRouter(NewConversationHandler(&fakeConversationServiceForHandler{
		getAllConversationsFn: func(ctx context.Context, filter service.ConversationAdminFilter) ([]service.ConversationAdminRecord, error) {
			if filter.UserID == nil || *filter.UserID != 7 {
				t.Fatalf("unexpected user filter: %+v", filter.UserID)
			}
			if filter.StartTime == nil || filter.EndTime == nil {
				t.Fatalf("expected date filters, got %+v", filter)
			}
			return []service.ConversationAdminRecord{{UserID: 7, Content: "hello", CreatedAt: time.Now()}}, nil
		},
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/conversation?userid=7&start_date=2026-01-01&end_date=2026-01-31", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expect 200, got %d, body=%s", w.Code, w.Body.String())
	}
}
