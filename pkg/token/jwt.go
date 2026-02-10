package token

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenType 常量，用于区分访问令牌和刷新令牌
// 防止攻击者拿 refresh token 冒充 access token 来访问 API
const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

// JWTManager 是 JWT 管理器，负责生成和验证 JWT
type JWTManager struct {
	secretKey            []byte        // 密钥
	accessTokenDuration  time.Duration // 访问令牌过期时间
	refreshTokenDuration time.Duration // 刷新令牌过期时间
}

// CustomClaims 是自定义的 Claims，包含用户信息和 JWT 标准 Claims
// 嵌入了 jwt.RegisteredClaims，所以 CustomClaims 也包含了 JWT 标准 Claims
type CustomClaims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	// TokenType 用于区分 access 和 refresh token，防止 token 类型混用攻击
	TokenType string `json:"token_type"`
	jwt.RegisteredClaims
}

// NewJWTManager 创建一个新的 JWTManager
// secretKey 是 JWT 的密钥
// accessTokenDuration 是访问令牌的过期时间
// refreshTokenDuration 是刷新令牌的过期时间
func NewJWTManager(secretKey string, accessTokenDuration, refreshTokenDuration time.Duration) *JWTManager {
	return &JWTManager{
		secretKey:            []byte(secretKey),
		accessTokenDuration:  accessTokenDuration,
		refreshTokenDuration: refreshTokenDuration,
	}
}

// GenerateToken 生成访问令牌和刷新令牌
func (manager *JWTManager) GenerateToken(userID uint, username, role string) (string, string, error) {
	// 当前时间
	now := time.Now()
	// 访问令牌过期时间
	exp := now.Add(manager.accessTokenDuration)
	// 访问令牌 Claims
	accessClaims := &CustomClaims{
		UserID:    userID,
		Username:  username,
		Role:      role,
		TokenType: TokenTypeAccess, // 标记为访问令牌，防止与刷新令牌混用
		// 访问令牌标准 Claims
		RegisteredClaims: jwt.RegisteredClaims{
			// 颁发者
			Issuer: "paismart",
			// 过期时间
			ExpiresAt: jwt.NewNumericDate(exp),
			// 颁发时间
			IssuedAt: jwt.NewNumericDate(now),
			// 生效时间
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	// 创建访问令牌
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	// 签名访问令牌
	accessTokenString, err := accessToken.SignedString(manager.secretKey)
	if err != nil {
		return "", "", err
	}
	// 刷新令牌 Claims
	refreshClaims := &CustomClaims{
		UserID:    userID,
		Username:  username,
		Role:      role,
		TokenType: TokenTypeRefresh, // 标记为刷新令牌，防止与访问令牌混用
		RegisteredClaims: jwt.RegisteredClaims{
			// 颁发者
			Issuer: "paismart",
			// 过期时间
			ExpiresAt: jwt.NewNumericDate(now.Add(manager.refreshTokenDuration)),
			// 颁发时间
			IssuedAt: jwt.NewNumericDate(now),
			// 生效时间
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	// 创建刷新令牌
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	// 签名刷新令牌
	refreshTokenString, err := refreshToken.SignedString(manager.secretKey)
	if err != nil {
		return "", "", err
	}
	return accessTokenString, refreshTokenString, nil
}

// VerifyToken 验证令牌
// tokenString 是 JWT 字符串
// 返回 CustomClaims 和 error
func (manager *JWTManager) VerifyToken(tokenString string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		// 返回密钥
		return manager.secretKey, nil
	},
		// 使用 WithValidMethods 精确限制只允许 HS256 算法
		// 替代手动类型断言，防止算法篡改攻击（如 alg=none 或 alg=RS256）
		// 相比检查 *jwt.SigningMethodHMAC 类型，这里精确到只允许 HS256 一种算法
		jwt.WithValidMethods([]string{"HS256"}),
	)
	// 返回错误
	if err != nil {
		return nil, err
	}
	// 返回 CustomClaims
	return token.Claims.(*CustomClaims), nil
}

// GenerateRandomString 生成随机字符串
// length 是字符串长度
// 返回随机字符串
func GenerateRandomString(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		// 如果生成随机字符串失败，返回一个默认字符串
		return fmt.Sprintf("fallback%d", time.Now().UnixNano())
	}
	// 返回随机字符串
	return hex.EncodeToString(bytes)
}
