package handler

import (
	"net/http"
	"strconv"
	"strings"

	"pai_smart_go_v2/internal/service"

	"github.com/gin-gonic/gin"
)

type SearchHandler struct {
	searchService service.SearchService
}

func NewSearchHandler(searchService service.SearchService) *SearchHandler {
	return &SearchHandler{searchService: searchService}
}

func (h *SearchHandler) HybridSearch(c *gin.Context) {
	if h.searchService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"code":    http.StatusServiceUnavailable,
			"error":   http.StatusText(http.StatusServiceUnavailable),
			"message": "Search service is unavailable",
		})
		return
	}

	user, ok := getUserFromContext(c)
	if !ok {
		return
	}

	query := strings.TrimSpace(c.Query("query"))
	topK := 10
	if topKRaw := strings.TrimSpace(c.Query("topK")); topKRaw != "" {
		parsed, err := strconv.Atoi(topKRaw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    http.StatusBadRequest,
				"error":   http.StatusText(http.StatusBadRequest),
				"message": "Query parameter 'topK' must be an integer",
			})
			return
		}
		topK = parsed
	}

	results, err := h.searchService.HybridSearch(c.Request.Context(), query, topK, user)
	if err != nil {
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{
			"code":    status,
			"error":   http.StatusText(status),
			"message": msg,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "Search successful",
		"data":    results,
	})
}
