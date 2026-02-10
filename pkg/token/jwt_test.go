package token

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// 测试用的常量
const (
	testSecret   = "test-secret-key-for-jwt-testing"
	testUserID   = uint(1)
	testUsername = "testuser"
	testRole     = "admin"
)

// 创建一个测试用的 JWTManager
func newTestManager() *JWTManager {
	return NewJWTManager(testSecret, 15*time.Minute, 7*24*time.Hour)
}

// TestNewJWTManager: 测试 JWTManager 的创建

func TestNewJWTManager(t *testing.T) {
	manager := NewJWTManager("my-secret", 10*time.Minute, 24*time.Hour)

	if manager == nil {
		t.Fatal("NewJWTManager 返回了 nil")
	}
	if string(manager.secretKey) != "my-secret" {
		t.Errorf("secretKey 期望 %q, 实际 %q", "my-secret", string(manager.secretKey))
	}
	if manager.accessTokenDuration != 10*time.Minute {
		t.Errorf("accessTokenDuration 期望 %v, 实际 %v", 10*time.Minute, manager.accessTokenDuration)
	}
	if manager.refreshTokenDuration != 24*time.Hour {
		t.Errorf("refreshTokenDuration 期望 %v, 实际 %v", 24*time.Hour, manager.refreshTokenDuration)
	}
}

// TestGenerateToken: 测试 Token 生成

func TestGenerateToken(t *testing.T) {
	manager := newTestManager()

	accessToken, refreshToken, err := manager.GenerateToken(testUserID, testUsername, testRole)
	if err != nil {
		t.Fatalf("GenerateToken 失败: %v", err)
	}

	// token 不能为空
	if accessToken == "" {
		t.Error("accessToken 为空")
	}
	if refreshToken == "" {
		t.Error("refreshToken 为空")
	}

	// JWT 格式：三段用 . 分隔
	if parts := strings.Split(accessToken, "."); len(parts) != 3 {
		t.Errorf("accessToken 格式不正确, 期望3段, 实际 %d 段", len(parts))
	}
	if parts := strings.Split(refreshToken, "."); len(parts) != 3 {
		t.Errorf("refreshToken 格式不正确, 期望3段, 实际 %d 段", len(parts))
	}

	// access 和 refresh 应该不同
	if accessToken == refreshToken {
		t.Error("accessToken 和 refreshToken 不应该相同")
	}
}

// TestVerifyToken_Success: 测试正常验证

func TestVerifyToken_Success(t *testing.T) {
	manager := newTestManager()

	accessToken, _, err := manager.GenerateToken(testUserID, testUsername, testRole)
	if err != nil {
		t.Fatalf("GenerateToken 失败: %v", err)
	}

	// 验证 token
	claims, err := manager.VerifyToken(accessToken)
	if err != nil {
		t.Fatalf("VerifyToken 失败: %v", err)
	}

	// 检查自定义字段
	if claims.UserID != testUserID {
		t.Errorf("UserID 期望 %d, 实际 %d", testUserID, claims.UserID)
	}
	if claims.Username != testUsername {
		t.Errorf("Username 期望 %q, 实际 %q", testUsername, claims.Username)
	}
	if claims.Role != testRole {
		t.Errorf("Role 期望 %q, 实际 %q", testRole, claims.Role)
	}

	// 检查 TokenType: access token 应该标记为 "access"
	if claims.TokenType != TokenTypeAccess {
		t.Errorf("TokenType 期望 %q, 实际 %q", TokenTypeAccess, claims.TokenType)
	}

	// 检查标准字段
	if claims.Issuer != "paismart" {
		t.Errorf("Issuer 期望 %q, 实际 %q", "paismart", claims.Issuer)
	}
	if claims.ExpiresAt == nil {
		t.Error("ExpiresAt 不应该为 nil")
	}
}

// TestVerifyToken_Expired: 测试过期的 Token

func TestVerifyToken_Expired(t *testing.T) {
	// 创建一个 1 秒就过期的 manager
	manager := NewJWTManager(testSecret, 1*time.Millisecond, 1*time.Millisecond)

	accessToken, _, err := manager.GenerateToken(testUserID, testUsername, testRole)
	if err != nil {
		t.Fatalf("GenerateToken 失败: %v", err)
	}

	// 等待 token 过期
	time.Sleep(10 * time.Millisecond)

	// 验证应该失败
	_, err = manager.VerifyToken(accessToken)
	if err == nil {
		t.Error("过期的 token 应该验证失败, 但返回了 nil error")
	}
}

// TestVerifyToken_WrongSecret: 测试用错误密钥验证

func TestVerifyToken_WrongSecret(t *testing.T) {
	manager := newTestManager()

	accessToken, _, err := manager.GenerateToken(testUserID, testUsername, testRole)
	if err != nil {
		t.Fatalf("GenerateToken 失败: %v", err)
	}

	// 用不同的密钥创建另一个 manager
	wrongManager := NewJWTManager("wrong-secret-key", 15*time.Minute, 7*24*time.Hour)

	// 用错误密钥验证应该失败
	_, err = wrongManager.VerifyToken(accessToken)
	if err == nil {
		t.Error("用错误密钥验证应该失败, 但返回了 nil error")
	}
}

// TestVerifyToken_Tampered: 测试被篡改的 Token

func TestVerifyToken_Tampered(t *testing.T) {
	manager := newTestManager()

	accessToken, _, err := manager.GenerateToken(testUserID, testUsername, testRole)
	if err != nil {
		t.Fatalf("GenerateToken 失败: %v", err)
	}

	// 篡改 token（修改 payload 部分的一个字符）
	parts := strings.Split(accessToken, ".")
	// 修改 payload 中的一个字符
	tampered := parts[0] + "." + parts[1] + "x" + "." + parts[2]

	_, err = manager.VerifyToken(tampered)
	if err == nil {
		t.Error("篡改的 token 应该验证失败, 但返回了 nil error")
	}
}

// TestVerifyToken_InvalidFormat: 测试无效格式的 Token

func TestVerifyToken_InvalidFormat(t *testing.T) {
	manager := newTestManager()

	invalidTokens := []string{
		"",          // 空字符串
		"not-a-jwt", // 随意字符串
		"a.b",       // 只有两段
		"a.b.c.d",   // 四段
	}

	for _, token := range invalidTokens {
		_, err := manager.VerifyToken(token)
		if err == nil {
			t.Errorf("无效 token %q 应该验证失败, 但返回了 nil error", token)
		}
	}
}

// TestVerifyToken_WrongSigningMethod: 测试错误的签名算法
// WithValidMethods 只允许 HS256，所以 none 和其他算法都应该被拒绝

func TestVerifyToken_WrongSigningMethod(t *testing.T) {
	// 用 none 算法创建一个 token（绕过签名）
	claims := &CustomClaims{
		UserID:    testUserID,
		Username:  testUsername,
		Role:      testRole,
		TokenType: TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "paismart",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	// 用 none 签名方法创建 token
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("创建 none 签名 token 失败: %v", err)
	}

	// 验证应该失败（WithValidMethods 只允许 HS256，none 算法会被拒绝）
	manager := newTestManager()
	_, err = manager.VerifyToken(tokenString)
	if err == nil {
		t.Error("none 签名的 token 应该验证失败, 但返回了 nil error")
	}
}

// TestVerifyToken_RefreshToken: 测试 refresh token 也能正常验证

func TestVerifyToken_RefreshToken(t *testing.T) {
	manager := newTestManager()

	_, refreshToken, err := manager.GenerateToken(testUserID, testUsername, testRole)
	if err != nil {
		t.Fatalf("GenerateToken 失败: %v", err)
	}

	claims, err := manager.VerifyToken(refreshToken)
	if err != nil {
		t.Fatalf("VerifyToken refresh token 失败: %v", err)
	}

	if claims.UserID != testUserID {
		t.Errorf("UserID 期望 %d, 实际 %d", testUserID, claims.UserID)
	}

	// 检查 TokenType: refresh token 应该标记为 "refresh"
	if claims.TokenType != TokenTypeRefresh {
		t.Errorf("TokenType 期望 %q, 实际 %q", TokenTypeRefresh, claims.TokenType)
	}
}

// ============================================================
// TestVerifyToken_TokenTypeMismatch: 测试 token 类型混用防护
// 验证 refresh token 不能冒充 access token 使用
// ============================================================
func TestVerifyToken_TokenTypeMismatch(t *testing.T) {
	manager := newTestManager()

	// 生成 token
	accessToken, refreshToken, err := manager.GenerateToken(testUserID, testUsername, testRole)
	if err != nil {
		t.Fatalf("GenerateToken 失败: %v", err)
	}

	// access token 的 TokenType 应该是 "access"
	accessClaims, err := manager.VerifyToken(accessToken)
	if err != nil {
		t.Fatalf("VerifyToken access token 失败: %v", err)
	}
	if accessClaims.TokenType != TokenTypeAccess {
		t.Errorf("access token 的 TokenType 期望 %q, 实际 %q", TokenTypeAccess, accessClaims.TokenType)
	}

	// refresh token 的 TokenType 应该是 "refresh"
	refreshClaims, err := manager.VerifyToken(refreshToken)
	if err != nil {
		t.Fatalf("VerifyToken refresh token 失败: %v", err)
	}
	if refreshClaims.TokenType != TokenTypeRefresh {
		t.Errorf("refresh token 的 TokenType 期望 %q, 实际 %q", TokenTypeRefresh, refreshClaims.TokenType)
	}

	// 业务层应通过检查 TokenType 来防止 refresh token 冒充 access token
	// 例如: if claims.TokenType != TokenTypeAccess { return error }
}

// TestGenerateRandomString: 测试随机字符串生成

func TestGenerateRandomString(t *testing.T) {
	// 测试长度（hex 编码后长度是原始字节的 2 倍）
	s := GenerateRandomString(16)
	if len(s) != 32 {
		t.Errorf("期望长度 32, 实际 %d", len(s))
	}

	s = GenerateRandomString(32)
	if len(s) != 64 {
		t.Errorf("期望长度 64, 实际 %d", len(s))
	}

	// 测试唯一性（两次生成的字符串不应该相同）
	s1 := GenerateRandomString(16)
	s2 := GenerateRandomString(16)
	if s1 == s2 {
		t.Error("两次生成的随机字符串不应该相同")
	}
}
