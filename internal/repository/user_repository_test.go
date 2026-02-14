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

func newMockRepo(t *testing.T) (UserRepository, sqlmock.Sqlmock) {
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

	return NewUserRepository(gdb), mock
}

func userRows() *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "username", "password", "role", "org_tags", "primary_org", "created_at", "updated_at",
	}).AddRow(1, "alice", "hashed", "USER", "tag1", "org1", now, now)
}

func TestUserRepository_Create(t *testing.T) {
	repo, mock := newMockRepo(t)

	u := &model.User{Username: "alice", Password: "hashed", Role: "USER"}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `users`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	if err := repo.Create(u); err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUserRepository_FindByUsername(t *testing.T) {
	repo, mock := newMockRepo(t)

	mock.ExpectQuery("SELECT .* FROM `users` WHERE username = \\? ORDER BY .* LIMIT \\?").
		WithArgs("alice", 1).
		WillReturnRows(userRows())

	u, err := repo.FindByUsername("alice")
	if err != nil {
		t.Fatalf("FindByUsername() error: %v", err)
	}
	if u == nil || u.Username != "alice" {
		t.Fatalf("unexpected user: %+v", u)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUserRepository_FindByUsername_NotFound(t *testing.T) {
	repo, mock := newMockRepo(t)

	mock.ExpectQuery("SELECT .* FROM `users` WHERE username = \\? ORDER BY .* LIMIT \\?").
		WithArgs("missing", 1).
		WillReturnError(gorm.ErrRecordNotFound)

	u, err := repo.FindByUsername("missing")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got: %v", err)
	}
	if u != nil {
		t.Fatalf("expected nil user, got: %+v", u)
	}
}

func TestUserRepository_Update_RowsAffectedZero(t *testing.T) {
	repo, mock := newMockRepo(t)

	u := &model.User{ID: 99, Username: "alice", Role: "ADMIN", OrgTags: "tag2", PrimaryOrg: "org2"}

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `users` SET .* WHERE id = \\?").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err := repo.Update(u)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got: %v", err)
	}
}

func TestUserRepository_FindWithPagination(t *testing.T) {
	repo, mock := newMockRepo(t)

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(1))

	mock.ExpectQuery("SELECT .* FROM `users` ORDER BY ID ASC LIMIT \\?").
		WithArgs(10).
		WillReturnRows(userRows())

	users, total, err := repo.FindWithPagination(0, 10)
	if err != nil {
		t.Fatalf("FindWithPagination() error: %v", err)
	}
	if total != 1 || len(users) != 1 {
		t.Fatalf("unexpected result: total=%d len=%d", total, len(users))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUserRepository_FindByID(t *testing.T) {
	repo, mock := newMockRepo(t)

	mock.ExpectQuery("SELECT .* FROM `users` WHERE .*id.* = \\? ORDER BY .* LIMIT \\?").
		WithArgs(uint(1), 1).
		WillReturnRows(userRows())

	u, err := repo.FindByID(1)
	if err != nil {
		t.Fatalf("FindByID() error: %v", err)
	}
	if u == nil || u.ID != 1 {
		t.Fatalf("unexpected user: %+v", u)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// ============================================================
// 以下为补充的测试用例，覆盖纯逻辑分支和遗漏的方法
// ============================================================

func TestUserRepository_Create_Nil(t *testing.T) {
	repo, _ := newMockRepo(t)

	if err := repo.Create(nil); err == nil {
		t.Fatal("expected error for nil user, got nil")
	}
}

func TestUserRepository_Update_Nil(t *testing.T) {
	repo, _ := newMockRepo(t)

	if err := repo.Update(nil); err == nil {
		t.Fatal("expected error for nil user, got nil")
	}
}

func TestUserRepository_Update_ZeroID(t *testing.T) {
	repo, _ := newMockRepo(t)

	u := &model.User{Username: "alice", Role: "ADMIN"}
	if err := repo.Update(u); err == nil {
		t.Fatal("expected error for zero ID, got nil")
	}
}

func TestUserRepository_Update_Success(t *testing.T) {
	repo, mock := newMockRepo(t)

	u := &model.User{ID: 1, Username: "alice", Role: "ADMIN", OrgTags: "tag2", PrimaryOrg: "org2"}

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `users` SET .* WHERE id = \\?").
		WillReturnResult(sqlmock.NewResult(0, 1)) // 1 row affected
	mock.ExpectCommit()

	if err := repo.Update(u); err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUserRepository_FindAll(t *testing.T) {
	repo, mock := newMockRepo(t)

	mock.ExpectQuery("SELECT .* FROM `users` ORDER BY ID ASC").
		WillReturnRows(userRows())

	users, err := repo.FindAll()
	if err != nil {
		t.Fatalf("FindAll() error: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[0].Username != "alice" {
		t.Fatalf("expected username=alice, got %s", users[0].Username)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUserRepository_FindWithPagination_Empty(t *testing.T) {
	repo, mock := newMockRepo(t)

	// total=0 时应提前返回，不执行第二条 SELECT 查询
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(0))

	users, total, err := repo.FindWithPagination(0, 10)
	if err != nil {
		t.Fatalf("FindWithPagination() error: %v", err)
	}
	if total != 0 {
		t.Fatalf("expected total=0, got %d", total)
	}
	if len(users) != 0 {
		t.Fatalf("expected 0 users, got %d", len(users))
	}
	// 验证没有多余的 SQL 查询被执行
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
