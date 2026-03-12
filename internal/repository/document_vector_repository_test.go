package repository

import (
	"testing"
	"time"

	"pai_smart_go_v2/internal/model"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newMockDocumentVectorRepo(t *testing.T) (DocumentVectorRepository, sqlmock.Sqlmock) {
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

	return NewDocumentVectorRepository(gdb), mock
}

func documentVectorRows() *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "file_md5", "chunk_id", "text_content", "model_version",
		"user_id", "org_tag", "is_public", "created_at", "updated_at",
	}).
		AddRow(1, "md5v", 0, "chunk-0", "", uint(7), "team-a", false, now, now).
		AddRow(2, "md5v", 1, "chunk-1", "", uint(7), "team-a", false, now, now)
}

func TestDocumentVectorRepository_BatchCreate_Empty(t *testing.T) {
	repo, _ := newMockDocumentVectorRepo(t)
	if err := repo.BatchCreate(nil); err != nil {
		t.Fatalf("BatchCreate(nil) error: %v", err)
	}
}

func TestDocumentVectorRepository_BatchCreate(t *testing.T) {
	repo, mock := newMockDocumentVectorRepo(t)

	vectors := []model.DocumentVector{
		{FileMD5: "md5v", ChunkID: 0, TextContent: "chunk-0", UserID: 7, OrgTag: "team-a"},
		{FileMD5: "md5v", ChunkID: 1, TextContent: "chunk-1", UserID: 7, OrgTag: "team-a"},
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `document_vectors`").WillReturnResult(sqlmock.NewResult(1, 2))
	mock.ExpectCommit()

	if err := repo.BatchCreate(vectors); err != nil {
		t.Fatalf("BatchCreate() error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestDocumentVectorRepository_FindByFileMD5(t *testing.T) {
	repo, mock := newMockDocumentVectorRepo(t)

	mock.ExpectQuery("SELECT .* FROM `document_vectors` WHERE file_md5 = \\? ORDER BY chunk_id ASC").
		WithArgs("md5v").
		WillReturnRows(documentVectorRows())

	vectors, err := repo.FindByFileMD5("md5v")
	if err != nil {
		t.Fatalf("FindByFileMD5() error: %v", err)
	}
	if len(vectors) != 2 || vectors[0].ChunkID != 0 || vectors[1].ChunkID != 1 {
		t.Fatalf("unexpected vectors: %+v", vectors)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestDocumentVectorRepository_DeleteByFileMD5(t *testing.T) {
	repo, mock := newMockDocumentVectorRepo(t)

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `document_vectors` WHERE file_md5 = \\?").
		WithArgs("md5v").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	if err := repo.DeleteByFileMD5("md5v"); err != nil {
		t.Fatalf("DeleteByFileMD5() error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
