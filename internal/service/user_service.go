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
	// ErrOrgTagNotFound 组织标签不存在
	ErrOrgTagNotFound = errors.New("organization tag not found")
	// ErrOrgTagNotOwned 用户未持有目标组织标签
	ErrOrgTagNotOwned = errors.New("organization tag does not belong to user")
	// ErrOrgTagAlreadyExists 创建标签时 TagID 重复
	ErrOrgTagAlreadyExists = errors.New("organization tag already exists")
	// ErrOrgTagHasChildren 删除标签时仍存在子节点
	ErrOrgTagHasChildren = errors.New("organization tag has children")
	// ErrInvalidInput 输入参数非法（空字符串、非法分页参数等）
	ErrInvalidInput = errors.New("invalid input")
	// ErrInternal 内部错误（对外不暴露细节）
	ErrInternal = errors.New("internal server error")
)

// OrgTagDetailDTO 是管理员用户列表中组织标签的精简展示结构。
// 只返回前端列表需要的字段，避免把完整标签模型（审计字段等）都暴露出去。
type OrgTagDetailDTO struct {
	TagID string `json:"tagId"`
	Name  string `json:"name"`
}

// UserDetailDTO 是管理员用户列表中的单个用户展示结构。
// 与 model.User 区别：
// 1. OrgTags 从逗号分隔字符串转换为结构化数组，便于前端直接渲染。
// 2. Status 保留兼容字段（0=ADMIN,1=USER），兼容旧前端逻辑。
type UserDetailDTO struct {
	UserID     uint              `json:"userId"`
	Username   string            `json:"username"`
	Role       string            `json:"role"`
	OrgTags    []OrgTagDetailDTO `json:"orgTags"`
	PrimaryOrg string            `json:"primaryOrg"`
	Status     int               `json:"status"`
	CreatedAt  time.Time         `json:"createdAt"`
}

type UserService interface {
	Register(username, password string) (*model.User, error)
	Login(username, password string) (accessToken, refreshToken string, err error)
	GetProfile(username string) (*model.User, error)
	FindByID(userID uint) (*model.User, error)

	Logout(token string) error
	SetUserPrimaryOrg(userID uint, orgTagID string) error
	GetUserOrgTags(userID uint) ([]model.OrganizationTag, error)
	GetUserEffectiveOrgTags(userID uint) ([]model.OrganizationTag, error)
	ListUsers(page, size int) ([]UserDetailDTO, int64, error)
	AssignOrgTagsToUser(userID uint, orgTagIDs []string) error
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

// FindByID 根据用户 ID 获取用户。
// 该方法给中间件和管理员接口复用，统一了用户不存在/数据库异常的错误语义。
func (s *userService) FindByID(userID uint) (*model.User, error) {
	if s.userRepo == nil {
		return nil, ErrInternal
	}

	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		log.Errorf("FindByID: failed to query user %d: %v", userID, err)
		return nil, ErrInternal
	}
	if user == nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}

// ListUsers 分页返回用户列表（管理员接口使用）。
// 关键点：
// 1. 输入 page/size 做兜底，避免出现负分页导致的不可预测行为。
// 2. 一次性加载组织标签并建索引，避免对每个用户做 N+1 次标签查询。
// 3. OrgTags 字段转为结构化数组，前端无需再解析逗号字符串。
func (s *userService) ListUsers(page, size int) ([]UserDetailDTO, int64, error) {
	if s.userRepo == nil || s.orgTagRepo == nil {
		return nil, 0, ErrInternal
	}
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 10
	}

	offset := (page - 1) * size
	users, total, err := s.userRepo.FindWithPagination(offset, size)
	if err != nil {
		return nil, 0, err
	}
	if len(users) == 0 {
		return []UserDetailDTO{}, total, nil
	}

	allTags, err := s.orgTagRepo.FindAll()
	if err != nil {
		return nil, 0, err
	}
	tagByID := make(map[string]model.OrganizationTag, len(allTags))
	for _, tag := range allTags {
		tagByID[tag.TagID] = tag
	}

	result := make([]UserDetailDTO, 0, len(users))
	for _, u := range users {
		tagIDs := parseOrgTagIDs(u.OrgTags)
		orgTagDetails := make([]OrgTagDetailDTO, 0, len(tagIDs))
		for _, tagID := range tagIDs {
			tag, ok := tagByID[tagID]
			if !ok {
				// 容错处理：用户历史脏数据中可能出现已删除标签。
				// 这里选择跳过而不是让整个列表失败，保证管理端可用性。
				continue
			}
			orgTagDetails = append(orgTagDetails, OrgTagDetailDTO{
				TagID: tag.TagID,
				Name:  tag.Name,
			})
		}

		status := 1
		if strings.EqualFold(u.Role, "ADMIN") {
			status = 0
		}

		result = append(result, UserDetailDTO{
			UserID:     u.ID,
			Username:   u.Username,
			Role:       u.Role,
			OrgTags:    orgTagDetails,
			PrimaryOrg: u.PrimaryOrg,
			Status:     status,
			CreatedAt:  u.CreatedAt,
		})
	}
	return result, total, nil
}

// AssignOrgTagsToUser 为目标用户分配组织标签集合（管理员接口使用）。
// 规则：
// 1. 会去重和清理空白标签 ID。
// 2. 所有标签必须真实存在，避免写入悬挂引用。
// 3. 如果用户当前 PrimaryOrg 不在新集合中，自动切换为第一个标签（若有）。
func (s *userService) AssignOrgTagsToUser(userID uint, orgTagIDs []string) error {
	if s.userRepo == nil || s.orgTagRepo == nil {
		return ErrInternal
	}

	user, err := s.FindByID(userID)
	if err != nil {
		return err
	}

	normalizedIDs := normalizeOrgTagIDs(orgTagIDs)
	for _, tagID := range normalizedIDs {
		if _, err := s.orgTagRepo.FindByID(tagID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOrgTagNotFound
			}
			return err
		}
	}

	user.OrgTags = strings.Join(normalizedIDs, ",")
	if len(normalizedIDs) == 0 {
		user.PrimaryOrg = ""
	} else if !containsString(normalizedIDs, user.PrimaryOrg) {
		user.PrimaryOrg = normalizedIDs[0]
	}

	if err := s.userRepo.Update(user); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUserNotFound
		}
		return err
	}
	return nil
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

// normalizeOrgTagIDs 对管理员提交的标签列表做标准化处理：
// 1. trim 去空白
// 2. 跳过空值
// 3. 保持输入顺序去重
func normalizeOrgTagIDs(rawIDs []string) []string {
	if len(rawIDs) == 0 {
		return []string{}
	}

	result := make([]string, 0, len(rawIDs))
	seen := make(map[string]struct{}, len(rawIDs))
	for _, rawID := range rawIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

// containsString 判断切片中是否包含目标值。
func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
