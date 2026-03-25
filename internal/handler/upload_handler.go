package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"pai_smart_go_v2/internal/service"
	"pai_smart_go_v2/pkg/log"

	"github.com/gin-gonic/gin"
)

// UploadHandler 负责文件上传/下载相关 HTTP 接口。
// Handler 只做 HTTP 翻译，所有业务逻辑和存储交互封装在 UploadService 中。
type UploadHandler struct {
	uploadService service.UploadService
}

// NewUploadHandler 创建 UploadHandler 实例。
func NewUploadHandler(uploadService service.UploadService) *UploadHandler {
	return &UploadHandler{
		uploadService: uploadService,
	}
}

// SimpleUpload 处理简单文件上传请求。
// 路由：POST /api/v1/upload/simple
// 请求格式：multipart/form-data，字段 "file"（必选）、"orgTag"（可选）
// 流程：解析文件 → 调用 Service（MD5计算 + 秒传检查 + MinIO上传 + DB写入）→ 返回结果
func (h *UploadHandler) SimpleUpload(c *gin.Context) {
	// 1. 从中间件上下文获取当前登录用户
	user, ok := getUserFromContext(c)
	if !ok {
		return
	}

	// 2. 解析 multipart 表单中的文件
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		log.Errorf("解析上传文件失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"error":   "Bad Request",
			"message": "File is required (form field: 'file')",
		})
		return
	}
	defer file.Close()

	// 3. 可选参数：组织标签（文件归属哪个组织）
	orgTag := c.PostForm("orgTag")

	// 4. 调用 Service 层执行上传逻辑
	result, err := h.uploadService.SimpleUpload(
		c.Request.Context(),
		user.ID,
		orgTag,
		header.Filename,
		header.Size,
		file,
	)
	if err != nil {
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{
			"code":    status,
			"error":   http.StatusText(status),
			"message": msg,
		})
		return
	}

	// 5. 返回上传结果
	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "Upload successful",
		"data":    result,
	})
}

// Download 处理文件下载请求。
// 路由：GET /api/v1/documents/download?fileMd5=xxx
// 流程：调用 Service 获取文件流 → 设置响应头 → 流式返回文件内容
func (h *UploadHandler) Download(c *gin.Context) {
	// 1. 获取查询参数
	fileMD5 := c.Query("fileMd5")
	if fileMD5 == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"error":   "Bad Request",
			"message": "Query parameter 'fileMd5' is required",
		})
		return
	}

	// 2. 从中间件上下文获取当前登录用户
	user, ok := getUserFromContext(c)
	if !ok {
		return
	}

	// 3. 调用 Service 获取文件流（DB 查找 + MinIO GetObject 统一在 Service 内完成）
	result, err := h.uploadService.DownloadFile(c.Request.Context(), fileMD5, user.ID)
	if err != nil {
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{
			"code":    status,
			"error":   http.StatusText(status),
			"message": msg,
		})
		return
	}
	defer result.Reader.Close()

	// 4. 设置响应头，触发浏览器下载
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, result.FileName))
	c.Header("Content-Type", result.ContentType)
	c.Header("Content-Length", fmt.Sprintf("%d", result.Size))

	// 5. 流式写入响应体（不会把整个文件加载到内存）
	c.DataFromReader(http.StatusOK, result.Size, result.ContentType, result.Reader, nil)
}

// ========== 阶段七：分片上传 ==========

// CheckFile 检查文件是否已上传（秒传）或已上传了哪些分片（断点续传）。
// 路由：POST /api/v1/upload/check
// 请求体：JSON {"md5": "..."}
func (h *UploadHandler) CheckFile(c *gin.Context) {
	user, ok := getUserFromContext(c)
	if !ok {
		return
	}

	var req struct {
		MD5 string `json:"md5" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"error":   "Bad Request",
			"message": "Field 'md5' is required",
		})
		return
	}

	result, err := h.uploadService.CheckFile(c.Request.Context(), req.MD5, user.ID)
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
		"message": "OK",
		"data":    result,
	})
}

func (h *UploadHandler) GetUploadStatus(c *gin.Context) {
	user, ok := getUserFromContext(c)
	if !ok {
		return
	}

	fileMD5 := c.Query("fileMd5")
	if fileMD5 == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"error":   "Bad Request",
			"message": "Query parameter 'fileMd5' is required",
		})
		return
	}

	result, err := h.uploadService.GetUploadStatus(c.Request.Context(), fileMD5, user.ID)
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
		"message": "Upload status retrieved successfully",
		"data":    result,
	})
}

func (h *UploadHandler) GetSupportedTypes(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "Supported upload types retrieved successfully",
		"data":    h.uploadService.GetSupportedTypes(),
	})
}

func (h *UploadHandler) FastUpload(c *gin.Context) {
	user, ok := getUserFromContext(c)
	if !ok {
		return
	}

	var req struct {
		MD5 string `json:"md5" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"error":   "Bad Request",
			"message": "Field 'md5' is required",
		})
		return
	}

	result, err := h.uploadService.CheckFastUpload(c.Request.Context(), req.MD5, user.ID)
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
		"message": "Fast upload check completed successfully",
		"data":    result,
	})
}

// UploadChunk 上传单个分片。
// 路由：POST /api/v1/upload/chunk
// 请求格式：multipart/form-data
// 字段：fileMd5, fileName, totalSize, chunkIndex, orgTag, isPublic, file
func (h *UploadHandler) UploadChunk(c *gin.Context) {
	user, ok := getUserFromContext(c)
	if !ok {
		return
	}

	fileMD5 := c.PostForm("fileMd5")
	fileName := c.PostForm("fileName")
	totalSizeStr := c.PostForm("totalSize")
	chunkIndexStr := c.PostForm("chunkIndex")
	orgTag := c.PostForm("orgTag")
	isPublicStr := c.PostForm("isPublic")

	if fileMD5 == "" || fileName == "" || totalSizeStr == "" || chunkIndexStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"error":   "Bad Request",
			"message": "Fields fileMd5, fileName, totalSize, chunkIndex are required",
		})
		return
	}

	totalSize, err := strconv.ParseInt(totalSizeStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"error":   "Bad Request",
			"message": "Invalid totalSize",
		})
		return
	}

	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"error":   "Bad Request",
			"message": "Invalid chunkIndex",
		})
		return
	}

	isPublic := isPublicStr == "true" || isPublicStr == "1"

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		log.Errorf("UploadChunk: 解析上传分片失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"error":   "Bad Request",
			"message": "File is required (form field: 'file')",
		})
		return
	}
	defer file.Close()

	result, err := h.uploadService.UploadChunk(
		c.Request.Context(),
		fileMD5, fileName, totalSize, chunkIndex,
		file, header.Size,
		user.ID, orgTag, isPublic,
	)
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
		"message": "Chunk uploaded",
		"data":    result,
	})
}

// MergeChunks 合并所有分片为最终文件。
// 路由：POST /api/v1/upload/merge
// 请求体：JSON {"fileMd5": "...", "fileName": "..."}
func (h *UploadHandler) MergeChunks(c *gin.Context) {
	user, ok := getUserFromContext(c)
	if !ok {
		return
	}

	var req struct {
		FileMD5  string `json:"fileMd5" binding:"required"`
		FileName string `json:"fileName" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"error":   "Bad Request",
			"message": "Fields fileMd5 and fileName are required",
		})
		return
	}

	result, err := h.uploadService.MergeChunks(c.Request.Context(), req.FileMD5, req.FileName, user.ID)
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
		"message": "Merge successful",
		"data":    result,
	})
}
