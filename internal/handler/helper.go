package handler

import (
	"errors"
	"net/http"
	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/internal/service"
	"strings"

	"github.com/gin-gonic/gin"
)

// mapServiceError 把 Service 层哨兵错误转换为 HTTP 状态码和对外消息。
// 统一映射的价值：
// 1. Handler 不必散落大量 if/else 判断。
// 2. 对外返回口径稳定，避免泄露内部实现细节。
func mapServiceError(err error) (httpStatus int, message string) {
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		return http.StatusBadRequest, "Invalid request parameters"
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
	case errors.Is(err, service.ErrOrgTagAlreadyExists):
		return http.StatusConflict, "Organization tag already exists"
	case errors.Is(err, service.ErrOrgTagHasChildren):
		return http.StatusConflict, "Organization tag has child nodes"
	default:
		return http.StatusInternalServerError, "Internal server error"
	}
}

// parseOrgTagIDsForResponse 把数据库中的逗号分隔字符串转成数组。
// 同时会去除空白和重复项，避免前端收到脏数据。
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

// extractBearerToken 从 Authorization 请求头提取 Bearer Token。
// 期望格式：Authorization: Bearer <token>
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

// getUserFromContext 从 Gin 上下文中读取 AuthMiddleware 注入的用户对象。
// 如果上下文异常，该函数会直接写错误响应并返回 false，调用方只需 `if !ok { return }`。
func getUserFromContext(c *gin.Context) (*model.User, bool) {
	userVal, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    http.StatusUnauthorized,
			"error":   "Unauthorized",
			"message": "User not found in context",
		})
		return nil, false
	}

	user, ok := userVal.(*model.User)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"error":   "Internal server error",
			"message": "Failed to get user profile",
		})
		return nil, false
	}
	return user, true
}
