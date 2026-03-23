package repository

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"pai_smart_go_v2/internal/model"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-redis/redis/v8"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newMockUploadRepo(t *testing.T, rdb *redis.Client) (UploadRepository, sqlmock.Sqlmock) {
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

	return NewUploadRepository(gdb, rdb), mock
}

func fileUploadRows() *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "file_md5", "file_name", "total_size", "status",
		"user_id", "org_tag", "is_public", "merged_at", "created_at", "updated_at",
	}).AddRow(1, "md5v", "a.pdf", int64(10), 0, uint(2), "team-a", false, nil, now, now)
}

func chunkInfoRows() *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "file_md5", "chunk_index", "storage_path", "created_at",
	}).
		AddRow(1, "md5v", 0, "chunks/md5v/0", now).
		AddRow(2, "md5v", 2, "chunks/md5v/2", now)
}

func TestUploadBitmapKey(t *testing.T) {
	key := uploadBitmapKey(12, "abc")
	if key != "upload:12:abc" {
		t.Fatalf("unexpected key: %s", key)
	}
}

func TestUploadRepository_Create_Nil(t *testing.T) {
	repo, _ := newMockUploadRepo(t, nil)

	if err := repo.Create(nil); err == nil {
		t.Fatal("expected error for nil upload")
	}
}

func TestUploadRepository_Create(t *testing.T) {
	repo, mock := newMockUploadRepo(t, nil)

	upload := &model.FileUpload{
		FileMD5:   "md5v",
		FileName:  "a.pdf",
		TotalSize: 10,
		Status:    0,
		UserID:    2,
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `file_uploads`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	if err := repo.Create(upload); err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUploadRepository_FindByFileMD5AndUserID(t *testing.T) {
	repo, mock := newMockUploadRepo(t, nil)

	mock.ExpectQuery("SELECT .* FROM `file_uploads` WHERE file_md5 = \\? AND user_id = \\? ORDER BY .* LIMIT \\?").
		WithArgs("md5v", uint(2), 1).
		WillReturnRows(fileUploadRows())

	upload, err := repo.FindByFileMD5AndUserID("md5v", 2)
	if err != nil {
		t.Fatalf("FindByFileMD5AndUserID() error: %v", err)
	}
	if upload == nil || upload.FileMD5 != "md5v" {
		t.Fatalf("unexpected upload: %+v", upload)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUploadRepository_FindBatchByMD5s(t *testing.T) {
	repo, mock := newMockUploadRepo(t, nil)

	rows := sqlmock.NewRows([]string{
		"id", "file_md5", "file_name", "total_size", "status",
		"user_id", "org_tag", "is_public", "merged_at", "created_at", "updated_at",
	}).
		AddRow(1, "md5-a", "a.pdf", int64(10), 1, uint(2), "team-a", false, nil, time.Now(), time.Now()).
		AddRow(2, "md5-b", "b.pdf", int64(20), 1, uint(2), "team-b", true, nil, time.Now(), time.Now())

	mock.ExpectQuery("SELECT .* FROM `file_uploads` WHERE file_md5 IN \\(.+\\)").
		WithArgs("md5-a", "md5-b").
		WillReturnRows(rows)

	uploads, err := repo.FindBatchByMD5s([]string{"md5-a", "md5-b"})
	if err != nil {
		t.Fatalf("FindBatchByMD5s() error: %v", err)
	}
	if len(uploads) != 2 || uploads[0].FileMD5 != "md5-a" || uploads[1].FileMD5 != "md5-b" {
		t.Fatalf("unexpected uploads: %+v", uploads)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUploadRepository_FindBatchByMD5s_Empty(t *testing.T) {
	repo, mock := newMockUploadRepo(t, nil)

	uploads, err := repo.FindBatchByMD5s(nil)
	if err != nil {
		t.Fatalf("FindBatchByMD5s() error: %v", err)
	}
	if len(uploads) != 0 {
		t.Fatalf("expected empty uploads, got %+v", uploads)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUploadRepository_FindFilesByUserID(t *testing.T) {
	repo, mock := newMockUploadRepo(t, nil)

	mock.ExpectQuery("SELECT .* FROM `file_uploads` WHERE user_id = \\? AND status = \\? ORDER BY created_at DESC").
		WithArgs(uint(2), 1).
		WillReturnRows(fileUploadRows())

	uploads, err := repo.FindFilesByUserID(2)
	if err != nil {
		t.Fatalf("FindFilesByUserID() error: %v", err)
	}
	if len(uploads) != 1 || uploads[0].UserID != 2 {
		t.Fatalf("unexpected uploads: %+v", uploads)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUploadRepository_FindAccessibleFiles(t *testing.T) {
	repo, mock := newMockUploadRepo(t, nil)

	mock.ExpectQuery("SELECT .* FROM `file_uploads` WHERE status = \\? AND \\(user_id = \\? OR is_public = \\? OR org_tag IN \\(.+\\)\\) ORDER BY created_at DESC").
		WithArgs(1, uint(7), true, "team-a", "team-b").
		WillReturnRows(fileUploadRows())

	uploads, err := repo.FindAccessibleFiles(7, []string{"team-a", "team-b"})
	if err != nil {
		t.Fatalf("FindAccessibleFiles() error: %v", err)
	}
	if len(uploads) != 1 {
		t.Fatalf("unexpected uploads: %+v", uploads)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUploadRepository_DeleteFileUploadRecord(t *testing.T) {
	repo, mock := newMockUploadRepo(t, nil)

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `file_uploads` WHERE file_md5 = \\? AND user_id = \\?").
		WithArgs("md5v", uint(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.DeleteFileUploadRecord("md5v", 9); err != nil {
		t.Fatalf("DeleteFileUploadRecord() error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUploadRepository_FindByID(t *testing.T) {
	repo, mock := newMockUploadRepo(t, nil)

	mock.ExpectQuery("SELECT .* FROM `file_uploads` WHERE .*id.* = \\? ORDER BY .* LIMIT \\?").
		WithArgs(uint(1), 1).
		WillReturnRows(fileUploadRows())

	upload, err := repo.FindByID(1)
	if err != nil {
		t.Fatalf("FindByID() error: %v", err)
	}
	if upload == nil || upload.ID != 1 {
		t.Fatalf("unexpected upload: %+v", upload)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUploadRepository_UpdateFileUploadStatus(t *testing.T) {
	repo, mock := newMockUploadRepo(t, nil)
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `file_uploads` SET .* WHERE file_md5 = \\? AND user_id = \\?").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.UpdateFileUploadStatus("md5v", 2, 1, &now); err != nil {
		t.Fatalf("UpdateFileUploadStatus() error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUploadRepository_CreateChunkInfo_Nil(t *testing.T) {
	repo, _ := newMockUploadRepo(t, nil)

	if err := repo.CreateChunkInfo(nil); err == nil {
		t.Fatal("expected error for nil chunk")
	}
}

func TestUploadRepository_CreateChunkInfo(t *testing.T) {
	repo, mock := newMockUploadRepo(t, nil)

	chunk := &model.ChunkInfo{
		FileMD5:     "md5v",
		ChunkIndex:  0,
		StoragePath: "chunks/md5v/0",
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `chunk_infos`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	if err := repo.CreateChunkInfo(chunk); err != nil {
		t.Fatalf("CreateChunkInfo() error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUploadRepository_FindChunksByFileMD5(t *testing.T) {
	repo, mock := newMockUploadRepo(t, nil)

	mock.ExpectQuery("SELECT .* FROM `chunk_infos` WHERE file_md5 = \\? ORDER BY chunk_index ASC").
		WithArgs("md5v").
		WillReturnRows(chunkInfoRows())

	chunks, err := repo.FindChunksByFileMD5("md5v")
	if err != nil {
		t.Fatalf("FindChunksByFileMD5() error: %v", err)
	}
	if len(chunks) != 2 || chunks[0].ChunkIndex != 0 || chunks[1].ChunkIndex != 2 {
		t.Fatalf("unexpected chunks: %+v", chunks)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUploadRepository_RedisBitmapLifecycle(t *testing.T) {
	rdb := newFakeRedisClient(t)

	repo := NewUploadRepository(nil, rdb)
	ctx := context.Background()

	if err := repo.MarkChunkUploaded(ctx, "md5v", 9, 0); err != nil {
		t.Fatalf("MarkChunkUploaded(0) error: %v", err)
	}
	if err := repo.MarkChunkUploaded(ctx, "md5v", 9, 3); err != nil {
		t.Fatalf("MarkChunkUploaded(3) error: %v", err)
	}
	if err := repo.MarkChunkUploaded(ctx, "md5v", 9, 8); err != nil {
		t.Fatalf("MarkChunkUploaded(8) error: %v", err)
	}

	ok, err := repo.IsChunkUploaded(ctx, "md5v", 9, 3)
	if err != nil {
		t.Fatalf("IsChunkUploaded(3) error: %v", err)
	}
	if !ok {
		t.Fatalf("expect chunk 3 uploaded")
	}

	ok, err = repo.IsChunkUploaded(ctx, "md5v", 9, 2)
	if err != nil {
		t.Fatalf("IsChunkUploaded(2) error: %v", err)
	}
	if ok {
		t.Fatalf("expect chunk 2 not uploaded")
	}

	uploaded, err := repo.GetUploadedChunksFromRedis(ctx, "md5v", 9, 10)
	if err != nil {
		t.Fatalf("GetUploadedChunksFromRedis() error: %v", err)
	}
	if len(uploaded) != 3 || uploaded[0] != 0 || uploaded[1] != 3 || uploaded[2] != 8 {
		t.Fatalf("unexpected uploaded chunks: %+v", uploaded)
	}

	if err := repo.DeleteUploadMark(ctx, "md5v", 9); err != nil {
		t.Fatalf("DeleteUploadMark() error: %v", err)
	}

	uploaded, err = repo.GetUploadedChunksFromRedis(ctx, "md5v", 9, 10)
	if err != nil {
		t.Fatalf("GetUploadedChunksFromRedis(after delete) error: %v", err)
	}
	if len(uploaded) != 0 {
		t.Fatalf("expected empty chunks after delete, got %+v", uploaded)
	}
}

func TestUploadRepository_GetUploadedChunksFromRedis_KeyNotFound(t *testing.T) {
	rdb := newFakeRedisClient(t)

	repo := NewUploadRepository(nil, rdb)
	uploaded, err := repo.GetUploadedChunksFromRedis(context.Background(), "missing", 1, 6)
	if err != nil {
		t.Fatalf("GetUploadedChunksFromRedis() error: %v", err)
	}
	if len(uploaded) != 0 {
		t.Fatalf("expected empty chunks, got %+v", uploaded)
	}
}

type fakeRedisBackend struct {
	mu     sync.Mutex
	values map[string][]byte
}

func newFakeRedisClient(t *testing.T) *redis.Client {
	t.Helper()

	backend := &fakeRedisBackend{
		values: make(map[string][]byte),
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: "fake-redis",
		Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
			clientConn, serverConn := net.Pipe()
			go backend.handleConn(serverConn)
			return clientConn, nil
		},
	})
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

func (s *fakeRedisBackend) handleConn(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	for {
		args, err := readRESPArray(reader)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				_ = writeError(writer, "ERR protocol error")
				_ = writer.Flush()
			}
			return
		}
		if len(args) == 0 {
			_ = writeError(writer, "ERR empty command")
			_ = writer.Flush()
			continue
		}

		if err := s.execute(writer, args); err != nil {
			_ = writeError(writer, err.Error())
		}
		if err := writer.Flush(); err != nil {
			return
		}
	}
}

func (s *fakeRedisBackend) execute(writer *bufio.Writer, args []string) error {
	cmd := strings.ToLower(args[0])
	switch cmd {
	case "ping":
		return writeSimpleString(writer, "PONG")
	case "command":
		return writeArrayHeader(writer, 0)
	case "client", "select":
		return writeSimpleString(writer, "OK")
	case "setbit":
		if len(args) != 4 {
			return fmt.Errorf("ERR wrong number of arguments for 'setbit'")
		}
		offset, err := strconv.ParseInt(args[2], 10, 64)
		if err != nil {
			return err
		}
		value, err := strconv.ParseInt(args[3], 10, 64)
		if err != nil {
			return err
		}
		old, err := s.setBit(args[1], offset, value)
		if err != nil {
			return err
		}
		return writeInteger(writer, int64(old))
	case "getbit":
		if len(args) != 3 {
			return fmt.Errorf("ERR wrong number of arguments for 'getbit'")
		}
		offset, err := strconv.ParseInt(args[2], 10, 64)
		if err != nil {
			return err
		}
		bit := s.getBit(args[1], offset)
		return writeInteger(writer, int64(bit))
	case "get":
		if len(args) != 2 {
			return fmt.Errorf("ERR wrong number of arguments for 'get'")
		}
		val, ok := s.get(args[1])
		if !ok {
			return writeNilBulkString(writer)
		}
		return writeBulkBytes(writer, val)
	case "set":
		if len(args) < 3 {
			return fmt.Errorf("ERR wrong number of arguments for 'set'")
		}
		s.set(args[1], []byte(args[2]))
		return writeSimpleString(writer, "OK")
	case "expire":
		if len(args) != 3 {
			return fmt.Errorf("ERR wrong number of arguments for 'expire'")
		}
		if _, err := strconv.ParseInt(args[2], 10, 64); err != nil {
			return err
		}
		return writeInteger(writer, 1)
	case "mget":
		if len(args) < 2 {
			return fmt.Errorf("ERR wrong number of arguments for 'mget'")
		}
		if err := writeArrayHeader(writer, len(args)-1); err != nil {
			return err
		}
		for _, key := range args[1:] {
			val, ok := s.get(key)
			if !ok {
				if err := writeNilBulkString(writer); err != nil {
					return err
				}
				continue
			}
			if err := writeBulkBytes(writer, val); err != nil {
				return err
			}
		}
		return nil
	case "scan":
		if len(args) < 2 {
			return fmt.Errorf("ERR wrong number of arguments for 'scan'")
		}
		cursor, err := strconv.ParseUint(args[1], 10, 64)
		if err != nil {
			return err
		}
		if cursor != 0 {
			return writeArrayOfBulkStrings(writer, "0")
		}

		matchPattern := "*"
		for i := 2; i < len(args)-1; i++ {
			if strings.EqualFold(args[i], "match") {
				matchPattern = args[i+1]
			}
		}
		return writeScanReply(writer, "0", s.keys(matchPattern))
	case "del":
		if len(args) < 2 {
			return fmt.Errorf("ERR wrong number of arguments for 'del'")
		}
		var deleted int64
		for _, key := range args[1:] {
			if s.del(key) {
				deleted++
			}
		}
		return writeInteger(writer, deleted)
	default:
		return fmt.Errorf("ERR unknown command '%s'", cmd)
	}
}

func (s *fakeRedisBackend) get(key string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	val, ok := s.values[key]
	if !ok {
		return nil, false
	}
	cp := make([]byte, len(val))
	copy(cp, val)
	return cp, true
}

func (s *fakeRedisBackend) del(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.values[key]
	if ok {
		delete(s.values, key)
	}
	return ok
}

func (s *fakeRedisBackend) set(key string, value []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cp := make([]byte, len(value))
	copy(cp, value)
	s.values[key] = cp
}

func (s *fakeRedisBackend) keys(matchPattern string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	keys := make([]string, 0, len(s.values))
	for key := range s.values {
		matched, err := path.Match(matchPattern, key)
		if err != nil || !matched {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (s *fakeRedisBackend) getBit(key string, offset int64) int {
	if offset < 0 {
		return 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data := s.values[key]
	byteIdx := int(offset / 8)
	if byteIdx >= len(data) {
		return 0
	}
	bitIdx := uint(7 - (offset % 8))
	if (data[byteIdx]>>bitIdx)&1 == 1 {
		return 1
	}
	return 0
}

func (s *fakeRedisBackend) setBit(key string, offset int64, value int64) (int, error) {
	if offset < 0 {
		return 0, fmt.Errorf("ERR bit offset is not an integer or out of range")
	}
	if value != 0 && value != 1 {
		return 0, fmt.Errorf("ERR bit is not an integer or out of range")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data := s.values[key]
	byteIdx := int(offset / 8)
	bitIdx := uint(7 - (offset % 8))
	if len(data) <= byteIdx {
		expanded := make([]byte, byteIdx+1)
		copy(expanded, data)
		data = expanded
	}

	old := 0
	if (data[byteIdx]>>bitIdx)&1 == 1 {
		old = 1
	}

	mask := byte(1 << bitIdx)
	if value == 1 {
		data[byteIdx] |= mask
	} else {
		data[byteIdx] &^= mask
	}
	s.values[key] = data
	return old, nil
}

func readRESPArray(r *bufio.Reader) ([]string, error) {
	head, err := readLine(r)
	if err != nil {
		return nil, err
	}
	if len(head) == 0 || head[0] != '*' {
		return nil, fmt.Errorf("invalid array header")
	}

	count, err := strconv.Atoi(head[1:])
	if err != nil {
		return nil, err
	}
	args := make([]string, 0, count)
	for i := 0; i < count; i++ {
		l, err := readLine(r)
		if err != nil {
			return nil, err
		}
		if len(l) == 0 || l[0] != '$' {
			return nil, fmt.Errorf("invalid bulk string header")
		}
		size, err := strconv.Atoi(l[1:])
		if err != nil {
			return nil, err
		}
		payload := make([]byte, size+2)
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, err
		}
		if payload[size] != '\r' || payload[size+1] != '\n' {
			return nil, fmt.Errorf("invalid bulk string ending")
		}
		args = append(args, string(payload[:size]))
	}
	return args, nil
}

func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	return line, nil
}

func writeSimpleString(w *bufio.Writer, value string) error {
	_, err := fmt.Fprintf(w, "+%s\r\n", value)
	return err
}

func writeError(w *bufio.Writer, value string) error {
	_, err := fmt.Fprintf(w, "-%s\r\n", value)
	return err
}

func writeInteger(w *bufio.Writer, v int64) error {
	_, err := fmt.Fprintf(w, ":%d\r\n", v)
	return err
}

func writeArrayHeader(w *bufio.Writer, n int) error {
	_, err := fmt.Fprintf(w, "*%d\r\n", n)
	return err
}

func writeNilBulkString(w *bufio.Writer) error {
	_, err := w.WriteString("$-1\r\n")
	return err
}

func writeBulkBytes(w *bufio.Writer, b []byte) error {
	if _, err := fmt.Fprintf(w, "$%d\r\n", len(b)); err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	_, err := w.WriteString("\r\n")
	return err
}

func writeArrayOfBulkStrings(w *bufio.Writer, values ...string) error {
	if err := writeArrayHeader(w, len(values)); err != nil {
		return err
	}
	for _, value := range values {
		if err := writeBulkBytes(w, []byte(value)); err != nil {
			return err
		}
	}
	return nil
}

func writeScanReply(w *bufio.Writer, cursor string, keys []string) error {
	if err := writeArrayHeader(w, 2); err != nil {
		return err
	}
	if err := writeBulkBytes(w, []byte(cursor)); err != nil {
		return err
	}
	return writeArrayOfBulkStrings(w, keys...)
}
