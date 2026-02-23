package handler

import (
	"fmt"
	"net/http"
	"pai_smart_go_v2/internal/service"
	"pai_smart_go_v2/pkg/log"
	"pai_smart_go_v2/pkg/storage"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
)

// UploadHandler 负责文件上传/下载相关 HTTP 接口。
// 路由需挂载 AuthMiddleware，确保请求已认证。
type UploadHandler struct {
	uploadService service.UploadService
	bucketName    string // MinIO 桶名，下载时需要用来获取对象
}

// NewUploadHandler 创建 UploadHandler 实例。
func NewUploadHandler(uploadService service.UploadService, bucketName string) *UploadHandler {
	return &UploadHandler{
		uploadService: uploadService,
		bucketName:    bucketName,
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
// 流程：查找 DB 记录 → 从 MinIO 获取对象流 → 设置响应头 → 流式返回文件内容
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

	// 3. 查找文件记录
	upload, err := h.uploadService.GetFileForDownload(c.Request.Context(), fileMD5, user.ID)
	if err != nil {
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{
			"code":    status,
			"error":   http.StatusText(status),
			"message": msg,
		})
		return
	}

	// 4. 根据上传时的对象键格式，从 MinIO 获取文件流
	objectKey := fmt.Sprintf("uploads/%d/%s/%s", upload.UserID, upload.FileMD5, upload.FileName)

	object, err := storage.MinIOClient.GetObject(
		c.Request.Context(),
		h.bucketName,
		objectKey,
		minio.GetObjectOptions{},
	)
	if err != nil {
		log.Errorf("从 MinIO 获取文件失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"error":   "Internal Server Error",
			"message": "Failed to retrieve file",
		})
		return
	}
	defer object.Close()

	// 5. 获取对象信息（用于设置 Content-Length）
	objectInfo, err := object.Stat()
	if err != nil {
		log.Errorf("获取 MinIO 对象信息失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"error":   "Internal Server Error",
			"message": "Failed to retrieve file info",
		})
		return
	}

	// 6. 设置响应头，触发浏览器下载
	// Content-Disposition: 告诉浏览器以附件形式下载，并指定文件名
	// Content-Type: 使用对象存储中记录的 MIME 类型
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, upload.FileName))
	c.Header("Content-Type", objectInfo.ContentType)
	c.Header("Content-Length", fmt.Sprintf("%d", objectInfo.Size))

	// 7. 流式写入响应体（不会把整个文件加载到内存）
	c.DataFromReader(http.StatusOK, objectInfo.Size, objectInfo.ContentType, object, nil)
}
