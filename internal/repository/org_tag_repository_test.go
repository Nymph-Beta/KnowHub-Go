package repository

import (
	"errors"
	"testing"
	"time"

	"pai_smart_go_v2/internal/model"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newMockOrgTagRepo(t *testing.T) (OrganizationTagRepository, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	gdb, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error: %v", err)
	}

	return NewOrganizationTagRepository(gdb), mock
}

func orgTagRows(tagID string, parentTag interface{}) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"tag_id", "name", "description", "parent_tag", "created_by", "updated_by", "created_at", "updated_at",
	}).AddRow(tagID, "Tech", "Tech team", parentTag, "admin", "admin", now, now)
}

func TestOrganizationTagRepository_Create(t *testing.T) {
	repo, mock := newMockOrgTagRepo(t)

	tag := &model.OrganizationTag{
		TagID:       "tech",
		Name:        "Tech",
		Description: "Tech team",
		CreatedBy:   "admin",
		UpdatedBy:   "admin",
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `organization_tags`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	if err := repo.Create(tag); err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestOrganizationTagRepository_FindByID(t *testing.T) {
	repo, mock := newMockOrgTagRepo(t)

	mock.ExpectQuery("SELECT .* FROM `organization_tags` WHERE tag_id = \\? ORDER BY .* LIMIT \\?").
		WithArgs("tech", 1).
		WillReturnRows(orgTagRows("tech", nil))

	tag, err := repo.FindByID("tech")
	if err != nil {
		t.Fatalf("FindByID() error: %v", err)
	}
	if tag == nil || tag.TagID != "tech" {
		t.Fatalf("unexpected tag: %+v", tag)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestOrganizationTagRepository_FindByParentTag_Root(t *testing.T) {
	repo, mock := newMockOrgTagRepo(t)

	mock.ExpectQuery("SELECT .* FROM `organization_tags` WHERE parent_tag IS NULL ORDER BY tag_id ASC").
		WillReturnRows(orgTagRows("root-tech", nil))

	tags, err := repo.FindByParentTag(nil)
	if err != nil {
		t.Fatalf("FindByParentTag(nil) error: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestOrganizationTagRepository_Update_RowsAffectedZero(t *testing.T) {
	repo, mock := newMockOrgTagRepo(t)

	tag := &model.OrganizationTag{
		TagID:       "missing",
		Name:        "Missing",
		Description: "Missing",
		UpdatedBy:   "admin",
	}

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `organization_tags` SET .* WHERE tag_id = \\?").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err := repo.Update(tag)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got: %v", err)
	}
}

func TestOrganizationTagRepository_Delete_HasChildren(t *testing.T) {
	repo, mock := newMockOrgTagRepo(t)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `organization_tags` WHERE tag_id = \\? ORDER BY .* LIMIT \\?").
		WithArgs("tech", 1).
		WillReturnRows(orgTagRows("tech", "dept"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `organization_tags` WHERE parent_tag = \\?").
		WithArgs("tech").
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(2))
	mock.ExpectRollback()

	err := repo.Delete("tech")
	if !errors.Is(err, ErrOrgTagHasChildren) {
		t.Fatalf("expected ErrOrgTagHasChildren, got: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestOrganizationTagRepository_Delete_Success(t *testing.T) {
	repo, mock := newMockOrgTagRepo(t)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `organization_tags` WHERE tag_id = \\? ORDER BY .* LIMIT \\?").
		WithArgs("tech", 1).
		WillReturnRows(orgTagRows("tech", "dept"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `organization_tags` WHERE parent_tag = \\?").
		WithArgs("tech").
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(0))
	mock.ExpectExec("DELETE FROM `organization_tags` WHERE tag_id = \\?").
		WithArgs("tech").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.Delete("tech"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestOrganizationTagRepository_DeleteAndReparentChildren_Success(t *testing.T) {
	repo, mock := newMockOrgTagRepo(t)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `organization_tags` WHERE tag_id = \\? ORDER BY .* LIMIT \\?").
		WithArgs("team-a", 1).
		WillReturnRows(orgTagRows("team-a", "dept-x"))
	mock.ExpectExec("UPDATE `organization_tags` SET `parent_tag`=\\?,`updated_at`=\\? WHERE parent_tag = \\?").
		WithArgs("dept-x", sqlmock.AnyArg(), "team-a").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("DELETE FROM `organization_tags` WHERE tag_id = \\?").
		WithArgs("team-a").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.DeleteAndReparentChildren("team-a"); err != nil {
		t.Fatalf("DeleteAndReparentChildren() error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
