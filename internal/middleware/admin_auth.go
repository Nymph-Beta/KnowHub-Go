package middleware

import (
	"net/http"
	"pai_smart_go_v2/internal/model"

	"github.com/gin-gonic/gin"
)

// AdminAuthMiddleware 是管理员认证中间件，用于保护需要管理员权限才能访问的接口。
// 该中间件必须早 AuthMiddleware 之后执行，因为管理员权限依赖于用户身份认证。
func AdminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从 AuthMiddleware 中获取用户信息
		userVal, exists := c.Get("user")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    http.StatusUnauthorized,
				"message": "User not found in context",
			})
			return
		}
		// 类型断言：将 any 转换为 *model.User
		user, ok := userVal.(*model.User)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"code":    http.StatusInternalServerError,
				"message": "Failed to get user profile",
			})
			return
		}
		// 检查用户是否是管理员
		if user.Role != "ADMIN" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    http.StatusForbidden,
				"message": "Forbidden: Only admin can access this resource",
			})
			return
		}
		// 放行
		c.Next()
	}
}
