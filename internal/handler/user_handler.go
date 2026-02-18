package handler

import (
	"errors"
	"net/http"
	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/internal/service"
	"pai_smart_go_v2/pkg/log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// mapServiceError 将 Service 层哨兵错误映射为 HTTP 状态码和对外安全的提示信息。
// 未识别的错误统一返回 500，避免泄露内部实现细节。
func mapServiceError(err error) (httpStatus int, message string) {
	switch {
	case errors.Is(err, service.ErrInvalidCredentials):
		return http.StatusUnauthorized, "Invalid username or password"
	case errors.Is(err, service.ErrUserAlreadyExists):
		return http.StatusConflict, "User already exists"
	case errors.Is(err, service.ErrUserNotFound):
		return http.StatusNotFound, "User not found"
	case errors.Is(err, service.ErrOrgTagNotFound):
		return http.StatusNotFound, "Organization tag not found"
	case errors.Is(err, service.ErrOrgTagNotOwned):
		return http.StatusForbidden, "Organization tag does not belong to user"
	default:
		return http.StatusInternalServerError, "Internal server error"
	}
}

// UserHandler 负责处理所有与普通用户的 APi 请求
type UserHandler struct {
	userService service.UserService
}

// NewUserHandler 创建一个新的 UserHandler 实例
func NewUserHandler(userService service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

// RegisterRequest 定义用户注册 API 的请求体结构
type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginRequest 定义用户登录 API 的请求体结构
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// ProfileResponse 定义用户个人信息 API 的响应体结构
type ProfileResponse struct {
	ID         uint      `json:"id"`
	Username   string    `json:"username"`
	Role       string    `json:"role"`
	OrgTags    []string  `json:"orgTags"`
	PrimaryOrg string    `json:"primaryOrg"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type SetPrimaryOrgRequest struct {
	PrimaryOrg string `json:"primaryOrg" binding:"required"`
}

// Register 处理用户注册请求
func (h *UserHandler) Register(c *gin.Context) {
	var req RegisterRequest
	// 绑定并验证 JSON 请求体
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warnf("Register: failed to bind request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": "Invalid request body",
		})
		return
	}

	// 调用 Service 层注册用户
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

	// 返回成功响应（201 Created 更符合 RESTful 语义）
	c.JSON(http.StatusCreated, gin.H{
		"code":    http.StatusCreated,
		"message": "User registered successfully",
		"data":    user,
	})
}

// Login 处理用户登录请求
func (h *UserHandler) Login(c *gin.Context) {
	var req LoginRequest
	// 绑定并验证 JSON 请求体
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warnf("Login: failed to bind request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": "Invalid request body",
		})
		return
	}

	// 调用 Service 层登录用户
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

	// 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "Login successful",
		"data": gin.H{
			"accessToken":  accessToken,
			"refreshToken": refreshToken,
		},
	})
}

// GetProfile 获取当前登录用户的个人信息。
// 用户信息已经由 AuthMiddleware 注入到上下文中。
func (h *UserHandler) GetProfile(c *gin.Context) {
	// 从上下文中获取由 AuthMiddleware 注入的 User 对象
	userVal, exists := c.Get("user")
	if !exists {
		log.Warnf("GetProfile: user not found in context")
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    http.StatusUnauthorized,
			"error":   "Unauthorized",
			"message": "User not found in context",
		})
		return
	}

	// 类型断言：将 any 转换为 *model.User
	user, ok := userVal.(*model.User)
	if !ok {
		log.Errorf("GetProfile: invalid user type in context")
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"error":   "Internal server error",
			"message": "Failed to get user profile",
		})
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

// Logout 处理用户退出登录请求
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

func (h *UserHandler) SetPrimaryOrg(c *gin.Context) {
	var req SetPrimaryOrgRequest
	// 绑定并验证 JSON 请求体
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warnf("SetPrimaryOrg: failed to bind request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": "Invalid request body",
		})
		return
	}

	// 从上下文中获取由 AuthMiddleware 注入的 User 对象
	userVal, exists := c.Get("user")
	if !exists {
		log.Warnf("SetPrimaryOrg: user not found in context")
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    http.StatusUnauthorized,
			"error":   "Unauthorized",
			"message": "User not found in context",
		})
		return
	}

	// 类型断言：将 any 转换为 *model.User
	user, ok := userVal.(*model.User)
	if !ok {
		log.Errorf("SetPrimaryOrg: invalid user type in context")
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"error":   "Internal server error",
			"message": "Failed to get user profile",
		})
		return
	}

	// 调用 Service 层设置主组织
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

	// 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "Primary org set successfully",
	})
}

// GetUserOrgTags 获取当前登录用户的组织标签
func (h *UserHandler) GetUserOrgTags(c *gin.Context) {
	// 从上下文中获取由 AuthMiddleware 注入的 User 对象
	userVal, exists := c.Get("user")
	if !exists {
		log.Warnf("GetUserOrgTags: user not found in context")
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    http.StatusUnauthorized,
			"error":   "Unauthorized",
			"message": "User not found in context",
		})
		return
	}

	// 类型断言：将 any 转换为 *model.User
	user, ok := userVal.(*model.User)
	if !ok {
		log.Errorf("GetUserOrgTags: invalid user type in context")
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"error":   "Internal server error",
			"message": "Failed to get user profile",
		})
		return
	}

	// 调用 Service 层获取用户组织标签
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

	// 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "User org tags retrieved successfully",
		"data":    orgTags,
	})
}

func parseOrgTagIDsForResponse(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	ids := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))

	for _, p := range parts {
		id := strings.TrimSpace(p)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func extractBearerToken(authHeader string) (string, error) {
	parts := strings.Fields(authHeader)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", errors.New("invalid authorization header")
	}
	if strings.TrimSpace(parts[1]) == "" {
		return "", errors.New("empty token")
	}
	return parts[1], nil
}
