package model

import "time"

// OrganizationTag 对应数据库中 organization_tags 表，表示组织标签。
// 组织标签支持树形结构，通过 ParentTag 指向父级标签实现层级关系。
type OrganizationTag struct {
	TagID       string    `gorm:"type:varchar(255);primaryKey" json:"tag_id"`
	Name        string    `gorm:"type:varchar(100);not null" json:"name"`
	Description string    `gorm:"type:varchar(255);not null" json:"description"`
	ParentTag   *string   `gorm:"type:varchar(255);index" json:"parent_tag"`
	CreatedBy   string    `gorm:"type:varchar(255);not null" json:"created_by"`
	UpdatedBy   string    `gorm:"type:varchar(255);not null" json:"updated_by"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// OrganizationTagNode 是组织标签的树形节点，用于构建前端需要的树形结构响应。
// 与 OrganizationTag（数据库模型）的区别：
//   - 不含 CreatedBy/UpdatedBy/CreatedAt/UpdatedAt 等审计字段
//   - 增加了 Children 字段，用于嵌套子节点
type OrganizationTagNode struct {
	TagID       string                 `json:"tagId"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	ParentTag   *string                `json:"parentTag"`
	Children    []*OrganizationTagNode `json:"children"`
}

// TableName 指定 GORM 使用的表名
func (OrganizationTag) TableName() string {
	return "organization_tags"
}
