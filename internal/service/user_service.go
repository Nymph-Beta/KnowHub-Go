package service

import (
	"context"
	"errors"
	"fmt"
	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/internal/repository"
	"pai_smart_go_v2/pkg/database"
	"pai_smart_go_v2/pkg/hash"
	"pai_smart_go_v2/pkg/log"
	"pai_smart_go_v2/pkg/token"
	"strings"
	"time"

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
	ErrInternal       = errors.New("internal server error")
	ErrOrgTagNotFound = errors.New("organization tag not found")
	ErrOrgTagNotOwned = errors.New("organization tag does not belong to user")
)

type UserService interface {
	Register(username, password string) (*model.User, error)
	Login(username, password string) (accessToken, refreshToken string, err error)
	GetProfile(username string) (*model.User, error)

	Logout(token string) error
	SetUserPrimaryOrg(userID uint, orgTagID string) error
	GetUserOrgTags(userID uint) ([]model.OrganizationTag, error)
	GetUserEffectiveOrgTags(userID uint) ([]model.OrganizationTag, error)
}

type userService struct {
	userRepo   repository.UserRepository
	orgTagRepo repository.OrganizationTagRepository
	JWTManager *token.JWTManager
}

func NewUserService(
	userRepo repository.UserRepository,
	orgTagRepo repository.OrganizationTagRepository,
	jwtManager *token.JWTManager,
) UserService {
	return &userService{
		userRepo:   userRepo,
		orgTagRepo: orgTagRepo,
		JWTManager: jwtManager,
	}
}

func (s *userService) Register(username, password string) (*model.User, error) {
	if s.userRepo == nil {
		return nil, ErrInternal
	}
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
	if err := s.userRepo.Create(newUser); err != nil {
		return nil, err
	}

	privateTagID := fmt.Sprintf("user:%d:private", newUser.ID)
	privateTag := &model.OrganizationTag{
		TagID:       privateTagID,
		Name:        fmt.Sprintf("%s Private", newUser.Username),
		Description: "Auto-created private organization tag",
		ParentTag:   nil,
		CreatedBy:   newUser.Username,
		UpdatedBy:   newUser.Username,
	}
	if err := s.orgTagRepo.Create(privateTag); err != nil {
		return nil, err
	}

	newUser.OrgTags = privateTagID
	newUser.PrimaryOrg = privateTagID
	if err := s.userRepo.Update(newUser); err != nil {
		return nil, err
	}

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

func (s *userService) Logout(token string) error {
	claims, err := s.JWTManager.VerifyToken(token)
	if err != nil {
		return ErrInvalidCredentials
	}
	userID := claims.UserID
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return ErrUserNotFound
	}
	if user == nil {
		return ErrUserNotFound
	}
	// 使用redis实现token黑名单，token剩余时间作为redis key的剩余时间
	if database.RDB == nil {
		return ErrInternal
	}
	redisKey := fmt.Sprintf("token_blacklist:%s", token)
	redisValue := fmt.Sprintf("%s:%s", claims.Username, claims.Role)
	if err := database.RDB.Set(context.Background(), redisKey, redisValue, claims.ExpiresAt.Sub(time.Now())).Err(); err != nil {
		return fmt.Errorf("failed to write token blacklist: %w", err)
	}
	return nil
}

func (s *userService) SetUserPrimaryOrg(userID uint, orgTagID string) error {
	if s.userRepo == nil {
		return ErrInternal
	}

	orgTagID = strings.TrimSpace(orgTagID)
	if orgTagID == "" {
		return ErrOrgTagNotFound
	}

	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUserNotFound
		}
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}
	owned := parseOrgTagIDs(user.OrgTags)
	ownedSet := make(map[string]struct{}, len(owned))
	for _, id := range owned {
		ownedSet[id] = struct{}{}
	}
	if _, ok := ownedSet[orgTagID]; !ok {
		return ErrOrgTagNotOwned
	}

	if _, err := s.orgTagRepo.FindByID(orgTagID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrOrgTagNotFound
		}
		return err
	}

	user.PrimaryOrg = orgTagID
	if err := s.userRepo.Update(user); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUserNotFound
		}
		return err
	}
	return nil
}

func (s *userService) GetUserOrgTags(userID uint) ([]model.OrganizationTag, error) {
	if s.orgTagRepo == nil {
		return nil, ErrInternal
	}
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	tagIDs := parseOrgTagIDs(user.OrgTags)
	if len(tagIDs) == 0 {
		return []model.OrganizationTag{}, nil
	}

	tags := make([]model.OrganizationTag, 0, len(tagIDs))
	for _, tagID := range tagIDs {
		tag, err := s.orgTagRepo.FindByID(tagID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrOrgTagNotFound
			}
			return nil, err
		}
		if tag == nil {
			return nil, ErrOrgTagNotFound
		}
		tags = append(tags, *tag)
	}
	return tags, nil
}

func (s *userService) GetUserEffectiveOrgTags(userID uint) ([]model.OrganizationTag, error) {
	if s.orgTagRepo == nil {
		return nil, ErrInternal
	}
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}
	seedIDs := parseOrgTagIDs(user.OrgTags)
	if len(seedIDs) == 0 {
		return []model.OrganizationTag{}, nil
	}

	allTags, err := s.orgTagRepo.FindAll()
	if err != nil {
		return nil, err
	}

	tagByID := make(map[string]model.OrganizationTag, len(allTags))
	children := make(map[string][]string)
	for _, t := range allTags {
		tagByID[t.TagID] = t
		if t.ParentTag != nil && *t.ParentTag != "" {
			children[*t.ParentTag] = append(children[*t.ParentTag], t.TagID)
		}
	}

	result := make([]model.OrganizationTag, 0, len(seedIDs))
	visited := make(map[string]struct{}, len(allTags))
	queue := append([]string(nil), seedIDs...)

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]

		if _, ok := visited[id]; ok {
			continue
		}
		visited[id] = struct{}{}

		tag, ok := tagByID[id]
		if !ok {
			return nil, ErrOrgTagNotFound
		}
		result = append(result, tag)

		for _, childID := range children[id] {
			if _, ok := visited[childID]; !ok {
				queue = append(queue, childID)
			}
		}
	}

	return result, nil
}

func parseOrgTagIDs(raw string) []string {
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
