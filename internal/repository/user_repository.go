package repository

import (
	"fmt"
	"pai_smart_go_v2/internal/model"

	"gorm.io/gorm"
)

// UserRepository 接口定义了用户数据的持久化操作。
type UserRepository interface {
	// 定义的用户数据的各个持久化操作
	Create(user *model.User) error
	FindByUsername(username string) (*model.User, error)
	Update(user *model.User) error
	FindAll() ([]model.User, error)
	FindWithPagination(offset, limit int) ([]model.User, int64, error)
	FindByID(userID uint) (*model.User, error)
}

// userRepository 是 UserRepository 接口的 GORM 实现。
type userRepository struct {
	db *gorm.DB
}

// NewUserRepository 创建一个新的 UserRepository 实例。
func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

// Create 创建一个新用户。
func (r *userRepository) Create(user *model.User) error {
	if user == nil {
		return fmt.Errorf("user is nil")
	}
	return r.db.Create(user).Error
}

// FindByUsername 根据用户名查找用户。
func (r *userRepository) FindByUsername(username string) (*model.User, error) {
	var user model.User
	if err := r.db.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// Update 更新用户信息。
func (r *userRepository) Update(user *model.User) error {
	if user == nil {
		return fmt.Errorf("user is nil")
	}
	if user.ID == 0 {
		return fmt.Errorf("user id is required")
	}
	tx := r.db.Model(&model.User{}).
		Where("id = ?", user.ID).
		Select("username", "role", "org_tags", "primary_org").
		Updates(user)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// FindAll 查找所有用户。
func (r *userRepository) FindAll() ([]model.User, error) {
	var users []model.User
	if err := r.db.Order("ID ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// FindWithPagination 分页查找用户。
func (r *userRepository) FindWithPagination(offset, limit int) ([]model.User, int64, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 20
	}

	var total int64
	if err := r.db.Model(&model.User{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if total == 0 {
		return []model.User{}, 0, nil
	}

	var users []model.User
	if err := r.db.Order("ID ASC").Offset(offset).Limit(limit).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

// FindByID 根据ID查找用户。
func (r *userRepository) FindByID(userID uint) (*model.User, error) {
	var user model.User
	if err := r.db.First(&user, userID).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
