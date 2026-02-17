package repository

import (
	"errors"
	"fmt"
	"pai_smart_go_v2/internal/model"

	"gorm.io/gorm"
)

var (
	// ErrOrgTagHasChildren 表示标签下仍有子节点，禁止直接删除。
	ErrOrgTagHasChildren = errors.New("organization tag has children")
)

// OrganizationTagRepository 组织标签仓库接口
type OrganizationTagRepository interface {
	// OrganizationTagRepository 定义组织标签的持久化操作接口。
	// 组织标签是树形结构，通过 ParentTag 实现父子关系。
	Create(tag *model.OrganizationTag) error
	FindAll() ([]model.OrganizationTag, error)
	FindByID(id string) (*model.OrganizationTag, error)
	FindByParentTag(parentTag *string) ([]model.OrganizationTag, error)
	// Update 更新标签信息（name, description, parent_tag, updated_by）
	Update(tag *model.OrganizationTag) error

	// Delete 默认保护删除：有子节点则返回 ErrOrgTagHasChildren
	// 使用事务保证"检查子节点 + 删除"的原子性。
	Delete(tagID string) error

	// DeleteAndReparentChildren 显式重挂删除：子节点挂到当前节点父节点后删除当前节点\// 然后删除当前节点。使用事务保证"重挂 + 删除"的原子性。
	// 示例：删除 B，B 的子节点 C、D 会挂到 B 的父节点 A 下。
	//   删除前：A → B → C, D
	//   删除后：A → C, D
	DeleteAndReparentChildren(tagID string) error
}

// organizationTagRepository 组织标签仓库实现
type organizationTagRepository struct {
	db *gorm.DB
}

func NewOrganizationTagRepository(db *gorm.DB) OrganizationTagRepository {
	return &organizationTagRepository{db: db}
}

func (r *organizationTagRepository) Create(tag *model.OrganizationTag) error {
	if tag == nil {
		return fmt.Errorf("tag is nil")
	}
	if tag.TagID == "" {
		return fmt.Errorf("tag id is required")
	}
	return r.db.Create(tag).Error
}

func (r *organizationTagRepository) FindAll() ([]model.OrganizationTag, error) {
	var tags []model.OrganizationTag
	if err := r.db.Order("tag_id ASC").Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}

func (r *organizationTagRepository) FindByID(id string) (*model.OrganizationTag, error) {
	if id == "" {
		return nil, fmt.Errorf("tag id is required")
	}

	var tag model.OrganizationTag
	if err := r.db.Where("tag_id = ?", id).First(&tag).Error; err != nil {
		return nil, err
	}
	return &tag, nil
}

func (r *organizationTagRepository) FindByParentTag(parentTag *string) ([]model.OrganizationTag, error) {
	var tags []model.OrganizationTag

	tx := r.db.Order("tag_id ASC")
	if parentTag == nil {
		tx = tx.Where("parent_tag IS NULL")
	} else {
		tx = tx.Where("parent_tag = ?", *parentTag)
	}

	if err := tx.Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}

// Update 更新标签的 name、description、parent_tag、updated_by 字段。
// 使用 Select 限定只更新这四个字段，避免零值覆盖其他字段。
// 如果 TagID 不存在，返回 gorm.ErrRecordNotFound。
func (r *organizationTagRepository) Update(tag *model.OrganizationTag) error {
	if tag == nil {
		return fmt.Errorf("organization tag is nil")
	}
	if tag.TagID == "" {
		return fmt.Errorf("tag id is required")
	}

	tx := r.db.Model(&model.OrganizationTag{}).
		Where("tag_id = ?", tag.TagID).
		Select("name", "description", "parent_tag", "updated_by").
		Updates(tag)

	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Delete 保护删除：在事务中先确认记录存在、再检查是否有子节点、最后执行删除。
// 有子节点时返回 ErrOrgTagHasChildren，调用方可据此提示用户先处理子节点。
func (r *organizationTagRepository) Delete(tagID string) error {
	if tagID == "" {
		return fmt.Errorf("tag id is required")
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		// 先确认记录存在
		var current model.OrganizationTag
		if err := tx.Where("tag_id = ?", tagID).First(&current).Error; err != nil {
			return err
		}

		// 保护删除：有子节点则拒绝
		var childCount int64
		if err := tx.Model(&model.OrganizationTag{}).
			Where("parent_tag = ?", tagID).
			Count(&childCount).Error; err != nil {
			return err
		}
		if childCount > 0 {
			return ErrOrgTagHasChildren
		}

		res := tx.Where("tag_id = ?", tagID).Delete(&model.OrganizationTag{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

// DeleteAndReparentChildren 重挂删除：在事务中先将子节点挂到当前节点的父节点下，
// 然后删除当前节点。适用于"删除中间层级但保留下级"的场景。
func (r *organizationTagRepository) DeleteAndReparentChildren(tagID string) error {
	if tagID == "" {
		return fmt.Errorf("tag id is required")
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		// 先查当前节点，拿到它的父节点（重挂目标）
		var current model.OrganizationTag
		if err := tx.Where("tag_id = ?", tagID).First(&current).Error; err != nil {
			return err
		}

		// 把直接子节点重挂到 current.ParentTag
		if err := tx.Model(&model.OrganizationTag{}).
			Where("parent_tag = ?", tagID).
			Update("parent_tag", current.ParentTag).Error; err != nil {
			return err
		}

		res := tx.Where("tag_id = ?", tagID).Delete(&model.OrganizationTag{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}
