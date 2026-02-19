package service

import (
	"errors"
	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/internal/repository"
	"strings"

	"gorm.io/gorm"
)

// OrgTagService 封装组织标签领域逻辑。
// 设计目标：
// 1. Handler 不直接操作 Repository，避免协议层混入业务规则。
// 2. 统一错误语义，把底层 gorm/repository 错误转换为 service 哨兵错误。
// 3. 聚合标签树构建、删除策略等“非纯 CRUD”的业务逻辑。
type OrgTagService interface {
	Create(tagID, name, description string, parentTag *string, actor string) (*model.OrganizationTag, error)
	Update(tagID, name, description string, parentTag *string, actor string) (*model.OrganizationTag, error)
	Delete(tagID string) error
	DeleteAndReparent(tagID string) error
	List() ([]model.OrganizationTag, error)
	GetTree() ([]*model.OrganizationTagNode, error)
	FindByID(tagID string) (*model.OrganizationTag, error)
}

type orgTagService struct {
	orgTagRepo repository.OrganizationTagRepository
}

func NewOrgTagService(orgTagRepo repository.OrganizationTagRepository) OrgTagService {
	return &orgTagService{orgTagRepo: orgTagRepo}
}

// Create 创建组织标签。
// 关键规则：
// 1. tagID/name 必填，且去除首尾空白。
// 2. tagID 不能重复。
// 3. 指定 parentTag 时，父标签必须存在。
func (s *orgTagService) Create(tagID, name, description string, parentTag *string, actor string) (*model.OrganizationTag, error) {
	if s.orgTagRepo == nil {
		return nil, ErrInternal
	}

	tagID = strings.TrimSpace(tagID)
	name = strings.TrimSpace(name)
	if tagID == "" || name == "" {
		return nil, ErrInvalidInput
	}

	normalizedParent := normalizeOptionalTagID(parentTag)
	if normalizedParent != nil && *normalizedParent == tagID {
		return nil, ErrInvalidInput
	}

	// 先检查当前 TagID 是否已存在，避免数据库唯一键报错直接外泄。
	_, err := s.orgTagRepo.FindByID(tagID)
	if err == nil {
		return nil, ErrOrgTagAlreadyExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// 指定父节点时，必须确保父节点存在，避免形成悬挂引用。
	if normalizedParent != nil {
		if _, err := s.orgTagRepo.FindByID(*normalizedParent); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrOrgTagNotFound
			}
			return nil, err
		}
	}

	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "system"
	}

	tag := &model.OrganizationTag{
		TagID:       tagID,
		Name:        name,
		Description: description,
		ParentTag:   normalizedParent,
		CreatedBy:   actor,
		UpdatedBy:   actor,
	}
	if err := s.orgTagRepo.Create(tag); err != nil {
		return nil, err
	}
	return tag, nil
}

// Update 更新组织标签字段。
// 关键规则：
// 1. 目标标签必须存在。
// 2. parentTag 允许置空（表示升为根节点）。
// 3. parentTag 不能指向自己。
func (s *orgTagService) Update(tagID, name, description string, parentTag *string, actor string) (*model.OrganizationTag, error) {
	if s.orgTagRepo == nil {
		return nil, ErrInternal
	}

	tagID = strings.TrimSpace(tagID)
	name = strings.TrimSpace(name)
	if tagID == "" || name == "" {
		return nil, ErrInvalidInput
	}

	tag, err := s.FindByID(tagID)
	if err != nil {
		return nil, err
	}

	normalizedParent := normalizeOptionalTagID(parentTag)
	if normalizedParent != nil && *normalizedParent == tagID {
		return nil, ErrInvalidInput
	}
	if normalizedParent != nil {
		if _, err := s.orgTagRepo.FindByID(*normalizedParent); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrOrgTagNotFound
			}
			return nil, err
		}
	}

	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "system"
	}

	tag.Name = name
	tag.Description = description
	tag.ParentTag = normalizedParent
	tag.UpdatedBy = actor

	if err := s.orgTagRepo.Update(tag); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrgTagNotFound
		}
		return nil, err
	}
	return tag, nil
}

// Delete 执行保护删除。
// 当标签有子节点时返回 ErrOrgTagHasChildren，提示调用方先处理层级关系。
func (s *orgTagService) Delete(tagID string) error {
	if s.orgTagRepo == nil {
		return ErrInternal
	}
	tagID = strings.TrimSpace(tagID)
	if tagID == "" {
		return ErrInvalidInput
	}

	if err := s.orgTagRepo.Delete(tagID); err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return ErrOrgTagNotFound
		case errors.Is(err, repository.ErrOrgTagHasChildren):
			return ErrOrgTagHasChildren
		default:
			return err
		}
	}
	return nil
}

// DeleteAndReparent 执行“重挂后删除”。
// 使用场景：删除中间层节点，但希望保留其下级节点。
func (s *orgTagService) DeleteAndReparent(tagID string) error {
	if s.orgTagRepo == nil {
		return ErrInternal
	}
	tagID = strings.TrimSpace(tagID)
	if tagID == "" {
		return ErrInvalidInput
	}

	if err := s.orgTagRepo.DeleteAndReparentChildren(tagID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrOrgTagNotFound
		}
		return err
	}
	return nil
}

func (s *orgTagService) List() ([]model.OrganizationTag, error) {
	if s.orgTagRepo == nil {
		return nil, ErrInternal
	}
	return s.orgTagRepo.FindAll()
}

// GetTree 构建标签树（根节点 + 递归 children）。
// 实现采用两遍扫描：
// 1. 第一遍创建所有节点并放入 map（tagID -> node）
// 2. 第二遍按 parent 关系把子节点挂到父节点上
func (s *orgTagService) GetTree() ([]*model.OrganizationTagNode, error) {
	if s.orgTagRepo == nil {
		return nil, ErrInternal
	}

	tags, err := s.orgTagRepo.FindAll()
	if err != nil {
		return nil, err
	}

	nodes := make(map[string]*model.OrganizationTagNode, len(tags))
	for _, tag := range tags {
		nodes[tag.TagID] = &model.OrganizationTagNode{
			TagID:       tag.TagID,
			Name:        tag.Name,
			Description: tag.Description,
			ParentTag:   tag.ParentTag,
			Children:    []*model.OrganizationTagNode{},
		}
	}

	tree := make([]*model.OrganizationTagNode, 0)
	for _, tag := range tags {
		node := nodes[tag.TagID]
		if tag.ParentTag != nil && *tag.ParentTag != "" {
			if parent, ok := nodes[*tag.ParentTag]; ok {
				parent.Children = append(parent.Children, node)
				continue
			}
		}
		// 父节点不存在或为空时，统一作为根节点返回，避免节点丢失。
		tree = append(tree, node)
	}
	return tree, nil
}

func (s *orgTagService) FindByID(tagID string) (*model.OrganizationTag, error) {
	if s.orgTagRepo == nil {
		return nil, ErrInternal
	}
	tagID = strings.TrimSpace(tagID)
	if tagID == "" {
		return nil, ErrInvalidInput
	}

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
	return tag, nil
}

// normalizeOptionalTagID 把可选字符串指针做标准化：
// 1. nil -> nil
// 2. 空白字符串 -> nil
// 3. 非空 -> trim 后返回新指针
func normalizeOptionalTagID(raw *string) *string {
	if raw == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
