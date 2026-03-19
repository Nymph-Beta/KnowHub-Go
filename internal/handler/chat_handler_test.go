package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"pai_smart_go_v2/internal/config"
	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/internal/service"
	"pai_smart_go_v2/pkg/token"

	"github.com/gin-gonic/gin"
)

type fakeChatService struct{}

func (f *fakeChatService) StreamResponse(ctx context.Context, question string, user *model.User, writer service.ChatResponseWriter, shouldStop func() bool) error {
	return nil
}

type fakeChatUserFinder struct {
	findByIDFn func(userID uint) (*model.User, error)
}

func (f *fakeChatUserFinder) FindByID(userID uint) (*model.User, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(userID)
	}
	return &model.User{ID: userID, Username: "alice"}, nil
}

func TestChatHandlerGetWebSocketTokenSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtManager := token.NewJWTManager("secret", time.Hour, 24*time.Hour)
	handler := NewChatHandler(&fakeChatService{}, &fakeChatUserFinder{}, jwtManager, config.LLMConfig{
		WebSocketTokenExpireMinutes: 3,
	})

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &model.User{ID: 7, Username: "alice", Role: "USER"})
		c.Next()
	})
	r.GET("/api/v1/chat/websocket-token", handler.GetWebSocketToken)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat/websocket-token", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			CmdToken string `json:"cmdToken"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.Data.CmdToken == "" {
		t.Fatal("expected cmdToken in response")
	}

	claims, err := jwtManager.VerifyToken(resp.Data.CmdToken)
	if err != nil {
		t.Fatalf("VerifyToken() error = %v", err)
	}
	if claims.TokenType != token.TokenTypeWebSocket || claims.UserID != 7 {
		t.Fatalf("unexpected websocket claims: %+v", claims)
	}
}

func TestChatHandlerGetWebSocketTokenUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewChatHandler(nil, &fakeChatUserFinder{}, token.NewJWTManager("secret", time.Hour, 24*time.Hour), config.LLMConfig{})

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &model.User{ID: 7, Username: "alice", Role: "USER"})
		c.Next()
	})
	r.GET("/api/v1/chat/websocket-token", handler.GetWebSocketToken)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat/websocket-token", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
}
