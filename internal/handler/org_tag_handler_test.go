package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/internal/service"

	"github.com/gin-gonic/gin"
)

type fakeOrgTagService struct {
	createFn            func(tagID, name, description string, parentTag *string, actor string) (*model.OrganizationTag, error)
	updateFn            func(tagID, name, description string, parentTag *string, actor string) (*model.OrganizationTag, error)
	deleteFn            func(tagID string) error
	deleteAndReparentFn func(tagID string) error
	listFn              func() ([]model.OrganizationTag, error)
	getTreeFn           func() ([]*model.OrganizationTagNode, error)
	findByIDFn          func(tagID string) (*model.OrganizationTag, error)
}

func (f *fakeOrgTagService) Create(tagID, name, description string, parentTag *string, actor string) (*model.OrganizationTag, error) {
	if f.createFn != nil {
		return f.createFn(tagID, name, description, parentTag, actor)
	}
	return nil, nil
}

func (f *fakeOrgTagService) Update(tagID, name, description string, parentTag *string, actor string) (*model.OrganizationTag, error) {
	if f.updateFn != nil {
		return f.updateFn(tagID, name, description, parentTag, actor)
	}
	return nil, nil
}

func (f *fakeOrgTagService) Delete(tagID string) error {
	if f.deleteFn != nil {
		return f.deleteFn(tagID)
	}
	return nil
}

func (f *fakeOrgTagService) DeleteAndReparent(tagID string) error {
	if f.deleteAndReparentFn != nil {
		return f.deleteAndReparentFn(tagID)
	}
	return nil
}

func (f *fakeOrgTagService) List() ([]model.OrganizationTag, error) {
	if f.listFn != nil {
		return f.listFn()
	}
	return []model.OrganizationTag{}, nil
}

func (f *fakeOrgTagService) GetTree() ([]*model.OrganizationTagNode, error) {
	if f.getTreeFn != nil {
		return f.getTreeFn()
	}
	return []*model.OrganizationTagNode{}, nil
}

func (f *fakeOrgTagService) FindByID(tagID string) (*model.OrganizationTag, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(tagID)
	}
	return nil, nil
}

func newOrgTagRouter(h *OrgTagHandler) *gin.Engine {
	r := gin.New()
	r.GET("/org-tags/tree", h.GetTree)
	r.DELETE("/org-tags/:id", h.Delete)
	return r
}

// Delete strategy: protect 分支应调用 Delete。
func TestOrgTagHandler_Delete_ProtectStrategy(t *testing.T) {
	calledDelete := false
	calledReparent := false
	svc := &fakeOrgTagService{
		deleteFn: func(tagID string) error {
			calledDelete = true
			if tagID != "team-a" {
				t.Fatalf("unexpected tagID: %s", tagID)
			}
			return nil
		},
		deleteAndReparentFn: func(tagID string) error {
			calledReparent = true
			return nil
		},
	}
	r := newOrgTagRouter(NewOrgTagHandler(svc))

	w := doReq(r, http.MethodDelete, "/org-tags/team-a?strategy=protect", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expect 200, got %d, body=%s", w.Code, w.Body.String())
	}
	if !calledDelete || calledReparent {
		t.Fatalf("expect only Delete called, got delete=%v reparent=%v", calledDelete, calledReparent)
	}
}

// Delete strategy: reparent 分支应调用 DeleteAndReparent。
func TestOrgTagHandler_Delete_ReparentStrategy(t *testing.T) {
	calledDelete := false
	calledReparent := false
	svc := &fakeOrgTagService{
		deleteFn: func(tagID string) error {
			calledDelete = true
			return nil
		},
		deleteAndReparentFn: func(tagID string) error {
			calledReparent = true
			if tagID != "team-a" {
				t.Fatalf("unexpected tagID: %s", tagID)
			}
			return nil
		},
	}
	r := newOrgTagRouter(NewOrgTagHandler(svc))

	w := doReq(r, http.MethodDelete, "/org-tags/team-a?strategy=reparent", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expect 200, got %d, body=%s", w.Code, w.Body.String())
	}
	if calledDelete || !calledReparent {
		t.Fatalf("expect only DeleteAndReparent called, got delete=%v reparent=%v", calledDelete, calledReparent)
	}
}

func TestOrgTagHandler_Delete_InvalidStrategy(t *testing.T) {
	svc := &fakeOrgTagService{}
	r := newOrgTagRouter(NewOrgTagHandler(svc))

	w := doReq(r, http.MethodDelete, "/org-tags/team-a?strategy=unknown", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expect 400, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestOrgTagHandler_Delete_ProtectHasChildren(t *testing.T) {
	svc := &fakeOrgTagService{
		deleteFn: func(tagID string) error {
			return service.ErrOrgTagHasChildren
		},
	}
	r := newOrgTagRouter(NewOrgTagHandler(svc))

	w := doReq(r, http.MethodDelete, "/org-tags/team-a?strategy=protect", "")
	if w.Code != http.StatusConflict {
		t.Fatalf("expect 409, got %d, body=%s", w.Code, w.Body.String())
	}
}

// GetTree 边界：当树为空时，Handler 仍返回 200，且 data 为数组而不是 null。
func TestOrgTagHandler_GetTree_EmptyList(t *testing.T) {
	svc := &fakeOrgTagService{
		getTreeFn: func() ([]*model.OrganizationTagNode, error) {
			return []*model.OrganizationTagNode{}, nil
		},
	}
	r := newOrgTagRouter(NewOrgTagHandler(svc))

	w := doReq(r, http.MethodGet, "/org-tags/tree", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expect 200, got %d, body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatalf("expect data to be array, got %T", resp["data"])
	}
	if len(data) != 0 {
		t.Fatalf("expect empty array, got %v", data)
	}
}

func TestOrgTagHandler_GetTree_ServiceError(t *testing.T) {
	svc := &fakeOrgTagService{
		getTreeFn: func() ([]*model.OrganizationTagNode, error) {
			return nil, errors.New("db down")
		},
	}
	r := newOrgTagRouter(NewOrgTagHandler(svc))

	w := doReq(r, http.MethodGet, "/org-tags/tree", "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expect 500, got %d, body=%s", w.Code, w.Body.String())
	}
}
