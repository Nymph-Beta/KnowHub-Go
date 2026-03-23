package handler

import (
	"net/http"
	"strconv"
	"time"

	"pai_smart_go_v2/internal/service"

	"github.com/gin-gonic/gin"
)

type ConversationHandler struct {
	conversationService service.ConversationService
}

func NewConversationHandler(conversationService service.ConversationService) *ConversationHandler {
	return &ConversationHandler{conversationService: conversationService}
}

func (h *ConversationHandler) GetConversations(c *gin.Context) {
	if h.conversationService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": http.StatusServiceUnavailable, "error": http.StatusText(http.StatusServiceUnavailable), "message": "Conversation service is unavailable"})
		return
	}
	user, ok := getUserFromContext(c)
	if !ok {
		return
	}

	history, err := h.conversationService.GetConversationHistory(c.Request.Context(), user.ID)
	if err != nil {
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{"code": status, "error": http.StatusText(status), "message": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "Conversation history retrieved successfully",
		"data":    history,
	})
}

func (h *ConversationHandler) GetAllConversations(c *gin.Context) {
	if h.conversationService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": http.StatusServiceUnavailable, "error": http.StatusText(http.StatusServiceUnavailable), "message": "Conversation service is unavailable"})
		return
	}
	filter, err := parseConversationAdminFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"error":   http.StatusText(http.StatusBadRequest),
			"message": err.Error(),
		})
		return
	}

	records, err := h.conversationService.GetAllConversations(c.Request.Context(), filter)
	if err != nil {
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{"code": status, "error": http.StatusText(status), "message": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "Conversations retrieved successfully",
		"data":    records,
	})
}

func parseConversationAdminFilter(c *gin.Context) (service.ConversationAdminFilter, error) {
	var filter service.ConversationAdminFilter

	if userIDRaw := c.Query("userid"); userIDRaw != "" {
		parsed, err := strconv.ParseUint(userIDRaw, 10, 64)
		if err != nil {
			return filter, err
		}
		userID := uint(parsed)
		filter.UserID = &userID
	}

	if startRaw := c.Query("start_date"); startRaw != "" {
		parsed, err := time.Parse("2006-01-02", startRaw)
		if err != nil {
			return filter, err
		}
		filter.StartTime = &parsed
	}

	if endRaw := c.Query("end_date"); endRaw != "" {
		parsed, err := time.Parse("2006-01-02", endRaw)
		if err != nil {
			return filter, err
		}
		parsed = parsed.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
		filter.EndTime = &parsed
	}

	return filter, nil
}
