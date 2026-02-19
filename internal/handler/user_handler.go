package handler

import (
	"net/http"
	"pai_smart_go_v2/internal/service"
	"pai_smart_go_v2/pkg/log"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// UserHandler 负责用户域相关 HTTP 接口。
// 注意：该 Handler 同时承载两类路由：
// 1. 普通用户路由（注册、登录、个人信息）
// 2. 管理员用户管理路由（列表、分配组织标签）
// 是否允许访问由路由组挂载的中间件决定，而不是靠 Handler 类型区分。
type UserHandler struct {
	userService service.UserService
}

// NewUserHandler 创建 UserHandler。
func NewUserHandler(userService service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

// RegisterRequest 是注册接口请求体。
type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginRequest 是登录接口请求体。
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// ProfileResponse 是个人信息接口响应结构。
// 与 model.User 的主要区别是 OrgTags 已转换为 string 数组。
type ProfileResponse struct {
	ID         uint      `json:"id"`
	Username   string    `json:"username"`
	Role       string    `json:"role"`
	OrgTags    []string  `json:"orgTags"`
	PrimaryOrg string    `json:"primaryOrg"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// SetPrimaryOrgRequest 是切换主组织接口请求体。
type SetPrimaryOrgRequest struct {
	PrimaryOrg string `json:"primaryOrg" binding:"required"`
}

// AssignOrgTagsRequest 是管理员为用户分配组织标签的请求体。
type AssignOrgTagsRequest struct {
	OrgTags []string `json:"orgTags" binding:"required"`
}

// Register 处理用户注册请求。
func (h *UserHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warnf("Register: failed to bind request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": "Invalid request body",
		})
		return
	}

	user, err := h.userService.Register(req.Username, req.Password)
	if err != nil {
		log.Warnf("Register: failed to register user: %v", err)
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{
			"code":    status,
			"message": msg,
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":    http.StatusCreated,
		"message": "User registered successfully",
		"data":    user,
	})
}

// Login 处理登录请求并返回 access/refresh token。
func (h *UserHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warnf("Login: failed to bind request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": "Invalid request body",
		})
		return
	}

	accessToken, refreshToken, err := h.userService.Login(req.Username, req.Password)
	if err != nil {
		log.Warnf("Login: failed to login user: %v", err)
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{
			"code":    status,
			"message": msg,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "Login successful",
		"data": gin.H{
			"accessToken":  accessToken,
			"refreshToken": refreshToken,
		},
	})
}

// GetProfile 返回当前登录用户信息。
// 用户对象由 AuthMiddleware 注入到上下文中。
func (h *UserHandler) GetProfile(c *gin.Context) {
	user, ok := getUserFromContext(c)
	if !ok {
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "Profile retrieved successfully",
		"data": ProfileResponse{
			ID:         user.ID,
			Username:   user.Username,
			Role:       user.Role,
			OrgTags:    parseOrgTagIDsForResponse(user.OrgTags),
			PrimaryOrg: user.PrimaryOrg,
			CreatedAt:  user.CreatedAt,
			UpdatedAt:  user.UpdatedAt,
		},
	})
}

// Logout 处理退出登录。
// 逻辑：从 Authorization 头提取 token，再交由 service 做黑名单处理。
func (h *UserHandler) Logout(c *gin.Context) {
	token, err := extractBearerToken(c.GetHeader("Authorization"))
	if err != nil {
		log.Warnf("Logout: invalid authorization header: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    http.StatusUnauthorized,
			"error":   "Unauthorized",
			"message": "Invalid authorization header",
		})
		return
	}

	err = h.userService.Logout(token)
	if err != nil {
		log.Warnf("Logout: failed to logout user: %v", err)
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{
			"code":    status,
			"message": msg,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "Logout successful",
	})
}

// SetPrimaryOrg 设置当前登录用户的主组织。
func (h *UserHandler) SetPrimaryOrg(c *gin.Context) {
	var req SetPrimaryOrgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warnf("SetPrimaryOrg: failed to bind request: %v", err)
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

	err := h.userService.SetUserPrimaryOrg(user.ID, req.PrimaryOrg)
	if err != nil {
		log.Warnf("SetPrimaryOrg: failed to set primary org: %v", err)
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{
			"code":    status,
			"message": msg,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "Primary org set successfully",
	})
}

// GetUserOrgTags 获取当前用户的直连组织标签。
func (h *UserHandler) GetUserOrgTags(c *gin.Context) {
	user, ok := getUserFromContext(c)
	if !ok {
		return
	}

	orgTags, err := h.userService.GetUserOrgTags(user.ID)
	if err != nil {
		log.Warnf("GetUserOrgTags: failed to get user org tags: %v", err)
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{
			"code":    status,
			"message": msg,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "User org tags retrieved successfully",
		"data":    orgTags,
	})
}

// ListUsers 管理员分页查询用户列表。
func (h *UserHandler) ListUsers(c *gin.Context) {
	pageRaw := c.DefaultQuery("page", "1")
	sizeRaw := c.DefaultQuery("size", "10")

	page, err := strconv.Atoi(pageRaw)
	if err != nil || page <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": "Invalid page parameter",
		})
		return
	}

	size, err := strconv.Atoi(sizeRaw)
	if err != nil || size <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": "Invalid size parameter",
		})
		return
	}

	users, total, err := h.userService.ListUsers(page, size)
	if err != nil {
		log.Warnf("ListUsers: failed to list users: %v", err)
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{
			"code":    status,
			"message": msg,
		})
		return
	}

	totalPages := 0
	if total > 0 {
		totalPages = (int(total) + size - 1) / size
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "Users retrieved successfully",
		"data": gin.H{
			"content":       users,
			"totalElements": total,
			"totalPages":    totalPages,
			"size":          size,
			"number":        page,
		},
	})
}

// AssignOrgTagsToUser 管理员为指定用户分配组织标签。
func (h *UserHandler) AssignOrgTagsToUser(c *gin.Context) {
	userID64, err := strconv.ParseUint(c.Param("userId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": "Invalid user ID",
		})
		return
	}

	var req AssignOrgTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": "Invalid request body",
		})
		return
	}

	if err := h.userService.AssignOrgTagsToUser(uint(userID64), req.OrgTags); err != nil {
		log.Warnf("AssignOrgTagsToUser: failed to assign org tags: %v", err)
		status, msg := mapServiceError(err)
		c.JSON(status, gin.H{
			"code":    status,
			"message": msg,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "Organization tags assigned successfully",
	})
}
