package service

import (
	"errors"
	"fmt"
	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/internal/repository"
	"pai_smart_go_v2/pkg/hash"
	"pai_smart_go_v2/pkg/log"
	"pai_smart_go_v2/pkg/token"

	"gorm.io/gorm"
)

// 哨兵错误：对外统一语义，隐藏底层实现细节
var (
	// ErrInvalidCredentials 用户名或密码错误（登录时统一返回，防止用户枚举）
	ErrInvalidCredentials = errors.New("invalid username or password")
	// ErrUserNotFound 用户不存在（仅用于非登录场景，如 GetProfile）
	ErrUserNotFound = errors.New("user not found")
	// ErrUserAlreadyExists 用户已存在（注册时）
	ErrUserAlreadyExists = errors.New("user already exists")
	// ErrInternal 内部错误（对外不暴露细节）
	ErrInternal = errors.New("internal server error")
)

type UserService interface {
	Register(username, password string) (*model.User, error)
	Login(username, password string) (accessToken, refreshToken string, err error)
	GetProfile(username string) (*model.User, error)
}

type userService struct {
	userRepo   repository.UserRepository
	JWTManager *token.JWTManager
}

func NewUserService(userRepo repository.UserRepository, jwtManager *token.JWTManager) UserService {
	return &userService{
		userRepo:   userRepo,
		JWTManager: jwtManager,
	}
}

func (s *userService) Register(username, password string) (*model.User, error) {
	// 1. 检查用户是否存在
	existingUser, err := s.userRepo.FindByUsername(username)
	if err != nil {
		// 查无记录是正常分支，继续注册
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	} else if existingUser != nil {
		return nil, ErrUserAlreadyExists
	}

	// 2. 密码进行哈希
	hashedPassword, err := hash.HashPassword(password)
	if err != nil {
		// 哈希失败是异常分支，直接返回错误
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// 3. 创建新用户
	newUser := &model.User{
		Username: username,
		Password: hashedPassword,
		Role:     "USER",
	}

	// 4. 用户存入数据库生成id
	err = s.userRepo.Create(newUser)
	if err != nil {
		return nil, err
	}

	// 5. 生成JWT令牌
	// accessToken, refreshToken, err = s.JWTManager.GenerateToken(newUser.ID, username, "USER")
	// if err != nil {
	// 	return nil, err
	// }

	return newUser, nil
}

func (s *userService) Login(username, password string) (accessToken, refreshToken string, err error) {
	if s.JWTManager == nil {
		return "", "", ErrInternal
	}
	// 1. 检查用户是否存在
	existingUser, err := s.userRepo.FindByUsername(username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 用户不存在，返回统一的凭证错误，防止用户枚举
			return "", "", ErrInvalidCredentials
		}
		// 真正的数据库错误：记日志，对外返回通用错误
		log.Errorf("Login: failed to query user %q: %v", username, err)
		return "", "", ErrInternal
	}
	if existingUser == nil {
		// 用户不存在，返回统一的凭证错误，防止用户枚举
		return "", "", ErrInvalidCredentials
	}

	// 2. 检查密码是否正确
	if !hash.CheckPasswordHash(password, existingUser.Password) {
		// 密码错误，返回与"用户不存在"相同的错误，防止用户枚举
		return "", "", ErrInvalidCredentials
	}

	// 3. 生成JWT令牌（使用数据库中的 Username，避免大小写/规范化不一致）
	accessToken, refreshToken, err = s.JWTManager.GenerateToken(existingUser.ID, existingUser.Username, existingUser.Role)
	if err != nil {
		log.Errorf("Login: failed to generate token for user %q: %v", existingUser.Username, err)
		return "", "", ErrInternal
	}
	return accessToken, refreshToken, nil
}

func (s *userService) GetProfile(username string) (*model.User, error) {
	user, err := s.userRepo.FindByUsername(username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		// 真正的数据库错误：记日志，对外返回通用错误
		log.Errorf("GetProfile: failed to query user %q: %v", username, err)
		return nil, ErrInternal
	}
	if user == nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}
