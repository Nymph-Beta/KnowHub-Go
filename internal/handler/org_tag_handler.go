package handler

import (
	"net/http"
	"pai_smart_go_v2/internal/service"
	"pai_smart_go_v2/pkg/log"
	"strings"

	"github.com/gin-gonic/gin"
)

// OrgTagHandler 负责组织标签管理接口（管理员路由）。
type OrgTagHandler struct {
	orgTagService service.OrgTagService
}

func NewOrgTagHandler(orgTagService service.OrgTagService) *OrgTagHandler {
	return &OrgTagHandler{orgTagService: orgTagService}
}

// CreateOrgTagRequest 是创建组织标签的请求体。
// parentTag 使用指针以区分“没传该字段”和“显式传空字符串”两种情况。
type CreateOrgTagRequest struct {
	TagID       string  `json:"tagId" binding:"required"`
	Name        string  `json:"name" binding:"required"`
	Description string  `json:"description"`
	ParentTag   *string `json:"parentTag"`
}

// UpdateOrgTagRequest 是更新组织标签的请求体。
type UpdateOrgTagRequest struct {
	Name        string  `json:"name" binding:"required"`
	Description string  `json:"description"`
	ParentTag   *string `json:"parentTag"`
}

// Create 创建组织标签。
func (h *OrgTagHandler) Create(c *gin.Context) {
	var req CreateOrgTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": "Invalid request body",
		})
		return
	}

	user, ok := getUserFromContext(c)
	if !ok {
		return
	}

	tag, err := h.orgTagService.Create(req.TagID, req.Name, req.Description, req.ParentTag, user.Username)
	if err != nil {
		log.Warnf("OrgTagHandler.Create: failed to create org tag: %v", err)
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{
			"code":    status,
			"message": msg,
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":    http.StatusCreated,
		"message": "Organization tag created successfully",
		"data":    tag,
	})
}

// List 返回组织标签平铺列表。
func (h *OrgTagHandler) List(c *gin.Context) {
	tags, err := h.orgTagService.List()
	if err != nil {
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{
			"code":    status,
			"message": msg,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "Organization tags retrieved successfully",
		"data":    tags,
	})
}

// GetTree 返回树形组织结构。
func (h *OrgTagHandler) GetTree(c *gin.Context) {
	tree, err := h.orgTagService.GetTree()
	if err != nil {
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{
			"code":    status,
			"message": msg,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "Organization tag tree retrieved successfully",
		"data":    tree,
	})
}

// Update 更新组织标签。
func (h *OrgTagHandler) Update(c *gin.Context) {
	tagID := c.Param("id")

	var req UpdateOrgTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": "Invalid request body",
		})
		return
	}

	user, ok := getUserFromContext(c)
	if !ok {
		return
	}

	tag, err := h.orgTagService.Update(tagID, req.Name, req.Description, req.ParentTag, user.Username)
	if err != nil {
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{
			"code":    status,
			"message": msg,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "Organization tag updated successfully",
		"data":    tag,
	})
}

// Delete 删除组织标签。
// 支持两种策略（通过 query 参数 strategy 控制）：
// 1. protect（默认）：有子节点时拒绝删除。
// 2. reparent：先重挂子节点，再删除当前节点。
func (h *OrgTagHandler) Delete(c *gin.Context) {
	tagID := c.Param("id")
	strategy := strings.ToLower(strings.TrimSpace(c.DefaultQuery("strategy", "protect")))

	var err error
	switch strategy {
	case "protect":
		err = h.orgTagService.Delete(tagID)
	case "reparent":
		err = h.orgTagService.DeleteAndReparent(tagID)
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": "Invalid delete strategy, use 'protect' or 'reparent'",
		})
		return
	}
	if err != nil {
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{
			"code":    status,
			"message": msg,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "Organization tag deleted successfully",
	})
}
