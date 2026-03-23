package handler

import (
	"net/http"
	"strings"

	"pai_smart_go_v2/internal/service"
	"pai_smart_go_v2/pkg/log"

	"github.com/gin-gonic/gin"
)

type DocumentHandler struct {
	documentService service.DocumentService
}

func NewDocumentHandler(documentService service.DocumentService) *DocumentHandler {
	return &DocumentHandler{documentService: documentService}
}

func (h *DocumentHandler) ListAccessibleFiles(c *gin.Context) {
	if h.documentService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": http.StatusServiceUnavailable, "error": http.StatusText(http.StatusServiceUnavailable), "message": "Document service is unavailable"})
		return
	}
	user, ok := getUserFromContext(c)
	if !ok {
		return
	}

	files, err := h.documentService.ListAccessibleFiles(c.Request.Context(), user)
	if err != nil {
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{"code": status, "error": http.StatusText(status), "message": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "Accessible files retrieved successfully",
		"data":    files,
	})
}

func (h *DocumentHandler) ListUploadedFiles(c *gin.Context) {
	if h.documentService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": http.StatusServiceUnavailable, "error": http.StatusText(http.StatusServiceUnavailable), "message": "Document service is unavailable"})
		return
	}
	user, ok := getUserFromContext(c)
	if !ok {
		return
	}

	files, err := h.documentService.ListUploadedFiles(c.Request.Context(), user.ID)
	if err != nil {
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{"code": status, "error": http.StatusText(status), "message": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "Uploaded files retrieved successfully",
		"data":    files,
	})
}

func (h *DocumentHandler) DeleteDocument(c *gin.Context) {
	if h.documentService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": http.StatusServiceUnavailable, "error": http.StatusText(http.StatusServiceUnavailable), "message": "Document service is unavailable"})
		return
	}
	fileMD5 := strings.TrimSpace(c.Param("fileMd5"))
	if fileMD5 == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"error":   http.StatusText(http.StatusBadRequest),
			"message": "Path parameter 'fileMd5' is required",
		})
		return
	}

	user, ok := getUserFromContext(c)
	if !ok {
		return
	}

	if err := h.documentService.DeleteDocument(c.Request.Context(), fileMD5, user); err != nil {
		log.Warnf("DeleteDocument: user=%d md5=%s err=%v", user.ID, fileMD5, err)
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{"code": status, "error": http.StatusText(status), "message": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "Document deleted successfully",
	})
}

func (h *DocumentHandler) GenerateDownloadURL(c *gin.Context) {
	if h.documentService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": http.StatusServiceUnavailable, "error": http.StatusText(http.StatusServiceUnavailable), "message": "Document service is unavailable"})
		return
	}
	user, ok := getUserFromContext(c)
	if !ok {
		return
	}

	fileMD5 := strings.TrimSpace(c.Query("fileMd5"))
	fileName := strings.TrimSpace(c.Query("fileName"))
	if fileMD5 == "" && fileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"error":   http.StatusText(http.StatusBadRequest),
			"message": "Query parameter 'fileMd5' or 'fileName' is required",
		})
		return
	}

	info, err := h.documentService.GenerateDownloadURL(c.Request.Context(), fileMD5, fileName, user)
	if err != nil {
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{"code": status, "error": http.StatusText(status), "message": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "Download URL generated successfully",
		"data":    info,
	})
}

func (h *DocumentHandler) PreviewFile(c *gin.Context) {
	if h.documentService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": http.StatusServiceUnavailable, "error": http.StatusText(http.StatusServiceUnavailable), "message": "Document service is unavailable"})
		return
	}
	user, ok := getUserFromContext(c)
	if !ok {
		return
	}

	fileMD5 := strings.TrimSpace(c.Query("fileMd5"))
	fileName := strings.TrimSpace(c.Query("fileName"))
	if fileMD5 == "" && fileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"error":   http.StatusText(http.StatusBadRequest),
			"message": "Query parameter 'fileMd5' or 'fileName' is required",
		})
		return
	}

	info, err := h.documentService.GetFilePreviewContent(c.Request.Context(), fileMD5, fileName, user)
	if err != nil {
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{"code": status, "error": http.StatusText(status), "message": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "Preview content retrieved successfully",
		"data":    info,
	})
}
