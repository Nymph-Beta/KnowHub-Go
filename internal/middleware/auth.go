package middleware

import (
	"context"
	"errors"
	"net/http"
	"pai_smart_go_v2/internal/service"
	"pai_smart_go_v2/pkg/database"
	"pai_smart_go_v2/pkg/token"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware 是 JWT 认证中间件，用于保护需要登录才能访问的接口。
// 工作流程：
//  1. 从请求头 Authorization 中提取 Bearer Token
//  2. 验证 Token 签名和有效期
//  3. 检查 Token 类型必须是 access（防止 refresh token 被滥用访问 API）
//  4. 检查 token 是否在 Redis 黑名单中（已登出 token 不再可用）
//  5. 根据 Token 中的用户名查询数据库，确认用户仍然存在
//  6. 将 claims 和 user 注入到 Gin 上下文中，后续 Handler 通过 c.Get("user") 获取
//
// 参数：
//   - jwtManager: JWT 管理器，负责验证 Token
//   - userService: 用户服务，用于查询用户是否存在
func AuthMiddleware(jwtManager *token.JWTManager, userService service.UserService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 0. 防御性检查：确保依赖已正确注入
		if jwtManager == nil || userService == nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"code":    http.StatusInternalServerError,
				"message": "Internal server error",
			})
			return
		}

		// 1. 从 Authorization 请求头中提取 Bearer Token
		//    格式要求：Authorization: Bearer <token>
		tokenString, err := extractBearerToken(c.GetHeader("Authorization"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    http.StatusUnauthorized,
				"message": "Invalid authorization header",
			})
			return
		}

		// 2. 验证 Token 的签名、有效期等
		claims, err := jwtManager.VerifyToken(tokenString)
		if err != nil || claims == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    http.StatusUnauthorized,
				"message": "Invalid or expired access token",
			})
			return
		}

		// 3. 检查 Token 类型：受保护接口只接受 access token，不接受 refresh token
		//    防止攻击者拿 refresh token 冒充 access token 来访问 API
		if claims.TokenType != token.TokenTypeAccess {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    http.StatusUnauthorized,
				"message": "Invalid token type",
			})
			return
		}

		// 4. 检查 Redis 黑名单：命中表示该 token 已被主动撤销（如用户登出）。
		// 这里与 Logout 使用同一 key 前缀，确保“写黑名单”和“读黑名单”一致。
		if database.RDB == nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"code":    http.StatusInternalServerError,
				"message": "Internal server error",
			})
			return
		}
		blacklistKey := "token_blacklist:" + tokenString
		exists, err := database.RDB.Exists(context.Background(), blacklistKey).Result()
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"code":    http.StatusInternalServerError,
				"message": "Internal server error",
			})
			return
		}
		if exists > 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    http.StatusUnauthorized,
				"message": "Invalid or expired access token",
			})
			return
		}

		// 5. 根据 Token 中的用户名查询数据库，确认用户仍然存在
		//    即使 Token 有效，用户也可能已被删除或禁用
		user, err := userService.GetProfile(claims.Username)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrUserNotFound):
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"code":    http.StatusUnauthorized,
					"message": "User not found",
				})
			default:
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"code":    http.StatusInternalServerError,
					"message": "Internal server error",
				})
			}
			return
		}
		if user == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    http.StatusUnauthorized,
				"message": "User not found",
			})
			return
		}

		// 6. 认证通过：将用户信息注入 Gin 上下文
		//    后续 Handler 通过 c.Get("claims") 获取 JWT Claims
		//    后续 Handler 通过 c.Get("user") 获取 *model.User（需类型断言）
		c.Set("claims", claims)
		c.Set("user", user)

		// 7. 调用下一个中间件或 Handler
		c.Next()
	}
}

// extractBearerToken 从 Authorization 请求头中提取 Bearer Token。
// 期望格式：Bearer <token>
// 使用 strings.EqualFold 做大小写不敏感比较，兼容 "bearer"、"BEARER" 等写法。
func extractBearerToken(authHeader string) (string, error) {
	// strings.Fields 按空白字符分割，自动处理多余空格
	parts := strings.Fields(authHeader)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", errors.New("invalid authorization header")
	}
	if parts[1] == "" {
		return "", errors.New("empty token")
	}
	return parts[1], nil
}
