package service

import (
	"errors"
	"testing"

	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/internal/repository"

	"gorm.io/gorm"
)

func strPtr(v string) *string {
	return &v
}

// TestOrgTagService_GetTree_OrphanAsRoot 验证 GetTree 的边界行为：
// 1. 正常父子关系应正确挂载到 children。
// 2. 父节点缺失（孤儿节点）不应丢失，应作为根节点返回。
func TestOrgTagService_GetTree_OrphanAsRoot(t *testing.T) {
	repo := &fakeOrgTagRepo{
		findAllFn: func() ([]model.OrganizationTag, error) {
			return []model.OrganizationTag{
				{TagID: "root", Name: "Root"},
				{TagID: "child", Name: "Child", ParentTag: strPtr("root")},
				{TagID: "orphan", Name: "Orphan", ParentTag: strPtr("missing-parent")},
			}, nil
		},
	}
	svc := NewOrgTagService(repo)

	tree, err := svc.GetTree()
	if err != nil {
		t.Fatalf("GetTree() error = %v", err)
	}
	if len(tree) != 2 {
		t.Fatalf("expect 2 root nodes (root + orphan), got %d", len(tree))
	}

	var rootNode *model.OrganizationTagNode
	var orphanNode *model.OrganizationTagNode
	for _, n := range tree {
		switch n.TagID {
		case "root":
			rootNode = n
		case "orphan":
			orphanNode = n
		}
	}

	if rootNode == nil {
		t.Fatalf("root node not found in tree: %+v", tree)
	}
	if len(rootNode.Children) != 1 || rootNode.Children[0].TagID != "child" {
		t.Fatalf("unexpected root children: %+v", rootNode.Children)
	}
	if orphanNode == nil {
		t.Fatalf("orphan node should be kept as root, tree=%+v", tree)
	}
	if len(orphanNode.Children) != 0 {
		t.Fatalf("orphan node should not have children, got %+v", orphanNode.Children)
	}
}

func TestOrgTagService_GetTree_RepoError(t *testing.T) {
	repo := &fakeOrgTagRepo{
		findAllFn: func() ([]model.OrganizationTag, error) {
			return nil, errors.New("db down")
		},
	}
	svc := NewOrgTagService(repo)

	_, err := svc.GetTree()
	if err == nil {
		t.Fatalf("expect error, got nil")
	}
}

func TestOrgTagService_Delete_HasChildrenMapped(t *testing.T) {
	repo := &fakeOrgTagRepo{
		deleteFn: func(tagID string) error {
			return repository.ErrOrgTagHasChildren
		},
	}
	svc := NewOrgTagService(repo)

	err := svc.Delete("team-a")
	if !errors.Is(err, ErrOrgTagHasChildren) {
		t.Fatalf("expect ErrOrgTagHasChildren, got %v", err)
	}
}

func TestOrgTagService_Delete_NotFoundMapped(t *testing.T) {
	repo := &fakeOrgTagRepo{
		deleteFn: func(tagID string) error {
			return gorm.ErrRecordNotFound
		},
	}
	svc := NewOrgTagService(repo)

	err := svc.Delete("missing")
	if !errors.Is(err, ErrOrgTagNotFound) {
		t.Fatalf("expect ErrOrgTagNotFound, got %v", err)
	}
}

func TestOrgTagService_DeleteAndReparent_NotFoundMapped(t *testing.T) {
	repo := &fakeOrgTagRepo{
		deleteAndReparentChildrenFn: func(tagID string) error {
			return gorm.ErrRecordNotFound
		},
	}
	svc := NewOrgTagService(repo)

	err := svc.DeleteAndReparent("missing")
	if !errors.Is(err, ErrOrgTagNotFound) {
		t.Fatalf("expect ErrOrgTagNotFound, got %v", err)
	}
}
