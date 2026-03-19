package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"pai_smart_go_v2/internal/config"
	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/internal/service"
	"pai_smart_go_v2/pkg/log"
	"pai_smart_go_v2/pkg/token"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type chatUserFinder interface {
	FindByID(userID uint) (*model.User, error)
}

type ChatHandler struct {
	chatService service.ChatService
	userService chatUserFinder
	jwtManager  *token.JWTManager
	llmCfg      config.LLMConfig
	upgrader    websocket.Upgrader
}

type chatClientMessage struct {
	Type         string `json:"type"`
	Content      string `json:"content"`
	CommandToken string `json:"_internal_cmd_token"`
}

type wsJSONWriter struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

type activeChatSession struct {
	commandToken string
	cancel       context.CancelFunc
}

func NewChatHandler(
	chatService service.ChatService,
	userService chatUserFinder,
	jwtManager *token.JWTManager,
	llmCfg config.LLMConfig,
) *ChatHandler {
	return &ChatHandler{
		chatService: chatService,
		userService: userService,
		jwtManager:  jwtManager,
		llmCfg:      llmCfg,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (h *ChatHandler) GetWebSocketToken(c *gin.Context) {
	if h.chatService == nil || h.jwtManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"code":    http.StatusServiceUnavailable,
			"error":   http.StatusText(http.StatusServiceUnavailable),
			"message": "Chat service is unavailable",
		})
		return
	}

	user, ok := getUserFromContext(c)
	if !ok {
		return
	}

	expireMinutes := h.llmCfg.WebSocketTokenExpireMinutes
	if expireMinutes <= 0 {
		expireMinutes = 5
	}

	cmdToken, err := h.jwtManager.GenerateWebSocketToken(user.ID, user.Username, user.Role, time.Duration(expireMinutes)*time.Minute)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"error":   http.StatusText(http.StatusInternalServerError),
			"message": "Failed to generate websocket token",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "WebSocket token generated",
		"data": gin.H{
			"cmdToken": cmdToken,
		},
	})
}

func (h *ChatHandler) HandleWebSocket(c *gin.Context) {
	if h.chatService == nil || h.userService == nil || h.jwtManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"code":    http.StatusServiceUnavailable,
			"error":   http.StatusText(http.StatusServiceUnavailable),
			"message": "Chat service is unavailable",
		})
		return
	}

	wsToken := strings.TrimSpace(c.Param("token"))
	claims, err := h.jwtManager.VerifyToken(wsToken)
	if err != nil || claims == nil || claims.TokenType != token.TokenTypeWebSocket {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    http.StatusUnauthorized,
			"error":   http.StatusText(http.StatusUnauthorized),
			"message": "Invalid websocket token",
		})
		return
	}

	user, err := h.userService.FindByID(claims.UserID)
	if err != nil {
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{
			"code":    status,
			"error":   http.StatusText(status),
			"message": msg,
		})
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Errorf("HandleWebSocket: upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	writer := &wsJSONWriter{conn: conn}
	var sessionMu sync.Mutex
	var active *activeChatSession

	clearActive := func(current *activeChatSession) {
		sessionMu.Lock()
		defer sessionMu.Unlock()
		if active == current {
			active = nil
		}
	}

	stopActive := func(commandToken string) bool {
		sessionMu.Lock()
		defer sessionMu.Unlock()
		current := active
		if current == nil {
			return false
		}
		if commandToken != "" && commandToken != current.commandToken {
			return false
		}
		current.cancel()
		return true
	}

	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			sessionMu.Lock()
			current := active
			active = nil
			sessionMu.Unlock()
			if current != nil {
				current.cancel()
			}
			return
		}

		var message chatClientMessage
		if err := jsonUnmarshal(payload, &message); err != nil {
			_ = writer.WriteJSON(gin.H{"error": "Invalid websocket message"})
			continue
		}

		switch strings.TrimSpace(message.Type) {
		case "stop":
			if !stopActive(strings.TrimSpace(message.CommandToken)) {
				_ = writer.WriteJSON(gin.H{"error": "No active generation to stop"})
			}
		case "message":
			content := strings.TrimSpace(message.Content)
			if content == "" {
				_ = writer.WriteJSON(gin.H{"error": "Message content is required"})
				continue
			}

			sessionMu.Lock()
			busy := active != nil
			sessionMu.Unlock()
			if busy {
				_ = writer.WriteJSON(gin.H{"error": "Generation is already in progress"})
				continue
			}

			streamCtx, cancel := context.WithCancel(c.Request.Context())
			current := &activeChatSession{
				commandToken: token.GenerateRandomString(12),
				cancel:       cancel,
			}

			sessionMu.Lock()
			active = current
			sessionMu.Unlock()

			if err := writer.WriteJSON(gin.H{
				"type":                "started",
				"status":              "streaming",
				"_internal_cmd_token": current.commandToken,
			}); err != nil {
				cancel()
				clearActive(current)
				continue
			}

			go func(current *activeChatSession, question string) {
				defer clearActive(current)

				shouldStop := func() bool {
					return streamCtx.Err() == context.Canceled
				}

				if err := h.chatService.StreamResponse(streamCtx, question, user, writer, shouldStop); err != nil {
					status, msg := mapServiceError(err)
					if status == http.StatusInternalServerError {
						msg = "Chat stream failed"
					}
					_ = writer.WriteJSON(gin.H{"error": msg})
				}
			}(current, content)
		default:
			_ = writer.WriteJSON(gin.H{"error": "Unsupported message type"})
		}
	}
}

func (w *wsJSONWriter) WriteJSON(v interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteJSON(v)
}

func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
