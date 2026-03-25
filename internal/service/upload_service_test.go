package service

import (
	"context"
	"errors"
	"io"
	"pai_smart_go_v2/pkg/tasks"
	"strings"
	"testing"
	"time"

	"pai_smart_go_v2/internal/model"

	"gorm.io/gorm"
)

type fakeUploadRepo struct {
	createFn                     func(upload *model.FileUpload) error
	findByFileMD5AndUserIDFn     func(fileMD5 string, userID uint) (*model.FileUpload, error)
	findBatchByMD5sFn            func(fileMD5s []string) ([]model.FileUpload, error)
	findFilesByUserIDFn          func(userID uint) ([]model.FileUpload, error)
	findAccessibleFilesFn        func(userID uint, orgTags []string) ([]model.FileUpload, error)
	findAccessibleFileByMD5Fn    func(userID uint, orgTags []string, fileMD5 string) (*model.FileUpload, error)
	findAccessibleFilesByNameFn  func(userID uint, orgTags []string, fileName string) ([]model.FileUpload, error)
	findByIDFn                   func(id uint) (*model.FileUpload, error)
	deleteFileUploadRecordFn     func(fileMD5 string, userID uint) error
	updateFileUploadStatusFn     func(fileMD5 string, userID uint, status int, mergedAt *time.Time) error
	updateFileProcessingStatusFn func(fileMD5 string, userID uint, processingStatus string) error
	createChunkInfoFn            func(chunk *model.ChunkInfo) error
	findChunksByFileMD5Fn        func(fileMD5 string) ([]model.ChunkInfo, error)
	deleteChunkInfosByFileMD5Fn  func(fileMD5 string) error
	isChunkUploadedFn            func(ctx context.Context, fileMD5 string, userID uint, chunkIndex int) (bool, error)
	markChunkUploadedFn          func(ctx context.Context, fileMD5 string, userID uint, chunkIndex int) error
	getUploadedChunksFromRedisFn func(ctx context.Context, fileMD5 string, userID uint, totalChunks int) ([]int, error)
	deleteUploadMarkFn           func(ctx context.Context, fileMD5 string, userID uint) error
}

func (f *fakeUploadRepo) Create(upload *model.FileUpload) error {
	if f.createFn != nil {
		return f.createFn(upload)
	}
	return nil
}

func (f *fakeUploadRepo) FindByFileMD5AndUserID(fileMD5 string, userID uint) (*model.FileUpload, error) {
	if f.findByFileMD5AndUserIDFn != nil {
		return f.findByFileMD5AndUserIDFn(fileMD5, userID)
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeUploadRepo) FindBatchByMD5s(fileMD5s []string) ([]model.FileUpload, error) {
	if f.findBatchByMD5sFn != nil {
		return f.findBatchByMD5sFn(fileMD5s)
	}
	return []model.FileUpload{}, nil
}

func (f *fakeUploadRepo) FindFilesByUserID(userID uint) ([]model.FileUpload, error) {
	if f.findFilesByUserIDFn != nil {
		return f.findFilesByUserIDFn(userID)
	}
	return []model.FileUpload{}, nil
}

func (f *fakeUploadRepo) FindAccessibleFiles(userID uint, orgTags []string) ([]model.FileUpload, error) {
	if f.findAccessibleFilesFn != nil {
		return f.findAccessibleFilesFn(userID, orgTags)
	}
	return []model.FileUpload{}, nil
}

func (f *fakeUploadRepo) FindAccessibleFileByMD5(userID uint, orgTags []string, fileMD5 string) (*model.FileUpload, error) {
	if f.findAccessibleFileByMD5Fn != nil {
		return f.findAccessibleFileByMD5Fn(userID, orgTags, fileMD5)
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeUploadRepo) FindAccessibleFilesByName(userID uint, orgTags []string, fileName string) ([]model.FileUpload, error) {
	if f.findAccessibleFilesByNameFn != nil {
		return f.findAccessibleFilesByNameFn(userID, orgTags, fileName)
	}
	return []model.FileUpload{}, nil
}

func (f *fakeUploadRepo) FindByID(id uint) (*model.FileUpload, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(id)
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeUploadRepo) DeleteFileUploadRecord(fileMD5 string, userID uint) error {
	if f.deleteFileUploadRecordFn != nil {
		return f.deleteFileUploadRecordFn(fileMD5, userID)
	}
	return nil
}

func (f *fakeUploadRepo) UpdateFileUploadStatus(fileMD5 string, userID uint, status int, mergedAt *time.Time) error {
	if f.updateFileUploadStatusFn != nil {
		return f.updateFileUploadStatusFn(fileMD5, userID, status, mergedAt)
	}
	return nil
}

func (f *fakeUploadRepo) UpdateFileProcessingStatus(fileMD5 string, userID uint, processingStatus string) error {
	if f.updateFileProcessingStatusFn != nil {
		return f.updateFileProcessingStatusFn(fileMD5, userID, processingStatus)
	}
	return nil
}

func (f *fakeUploadRepo) CreateChunkInfo(chunk *model.ChunkInfo) error {
	if f.createChunkInfoFn != nil {
		return f.createChunkInfoFn(chunk)
	}
	return nil
}

func (f *fakeUploadRepo) FindChunksByFileMD5(fileMD5 string) ([]model.ChunkInfo, error) {
	if f.findChunksByFileMD5Fn != nil {
		return f.findChunksByFileMD5Fn(fileMD5)
	}
	return []model.ChunkInfo{}, nil
}

func (f *fakeUploadRepo) DeleteChunkInfosByFileMD5(fileMD5 string) error {
	if f.deleteChunkInfosByFileMD5Fn != nil {
		return f.deleteChunkInfosByFileMD5Fn(fileMD5)
	}
	return nil
}

func (f *fakeUploadRepo) IsChunkUploaded(ctx context.Context, fileMD5 string, userID uint, chunkIndex int) (bool, error) {
	if f.isChunkUploadedFn != nil {
		return f.isChunkUploadedFn(ctx, fileMD5, userID, chunkIndex)
	}
	return false, nil
}

func (f *fakeUploadRepo) MarkChunkUploaded(ctx context.Context, fileMD5 string, userID uint, chunkIndex int) error {
	if f.markChunkUploadedFn != nil {
		return f.markChunkUploadedFn(ctx, fileMD5, userID, chunkIndex)
	}
	return nil
}

func (f *fakeUploadRepo) GetUploadedChunksFromRedis(ctx context.Context, fileMD5 string, userID uint, totalChunks int) ([]int, error) {
	if f.getUploadedChunksFromRedisFn != nil {
		return f.getUploadedChunksFromRedisFn(ctx, fileMD5, userID, totalChunks)
	}
	return []int{}, nil
}

func (f *fakeUploadRepo) DeleteUploadMark(ctx context.Context, fileMD5 string, userID uint) error {
	if f.deleteUploadMarkFn != nil {
		return f.deleteUploadMarkFn(ctx, fileMD5, userID)
	}
	return nil
}

type fakeUploadUserRepo struct {
	findByIDFn func(userID uint) (*model.User, error)
}

type fakeTaskProducer struct {
	produceFn func(ctx context.Context, task tasks.FileProcessingTask) error
	called    int
	lastTask  tasks.FileProcessingTask
}

func (f *fakeTaskProducer) ProduceFileTask(ctx context.Context, task tasks.FileProcessingTask) error {
	f.called++
	f.lastTask = task
	if f.produceFn != nil {
		return f.produceFn(ctx, task)
	}
	return nil
}

func (f *fakeUploadUserRepo) Create(user *model.User) error { return nil }
func (f *fakeUploadUserRepo) FindByUsername(username string) (*model.User, error) {
	return nil, gorm.ErrRecordNotFound
}
func (f *fakeUploadUserRepo) Update(user *model.User) error  { return nil }
func (f *fakeUploadUserRepo) FindAll() ([]model.User, error) { return []model.User{}, nil }
func (f *fakeUploadUserRepo) FindWithPagination(offset, limit int) ([]model.User, int64, error) {
	return []model.User{}, 0, nil
}
func (f *fakeUploadUserRepo) FindByID(userID uint) (*model.User, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(userID)
	}
	return &model.User{ID: userID, PrimaryOrg: "team-default"}, nil
}

func TestCalcTotalChunks(t *testing.T) {
	tests := []struct {
		totalSize int64
		want      int
	}{
		{totalSize: 1, want: 1},
		{totalSize: DefaultChunkSize, want: 1},
		{totalSize: DefaultChunkSize + 1, want: 2},
		{totalSize: DefaultChunkSize*2 + 123, want: 3},
	}

	for _, tt := range tests {
		got := calcTotalChunks(tt.totalSize)
		if got != tt.want {
			t.Fatalf("calcTotalChunks(%d)=%d, want=%d", tt.totalSize, got, tt.want)
		}
	}
}

func TestMakeRange(t *testing.T) {
	got := makeRange(4)
	want := []int{0, 1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got=%d want=%d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("makeRange mismatch at %d: got=%d want=%d", i, got[i], want[i])
		}
	}
}

func TestUploadService_CheckFile_NotFound(t *testing.T) {
	uploadRepo := &fakeUploadRepo{
		findByFileMD5AndUserIDFn: func(fileMD5 string, userID uint) (*model.FileUpload, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := NewUploadService(uploadRepo, &fakeUploadUserRepo{}, nil, "uploads", nil)

	result, err := svc.CheckFile(context.Background(), "md5-x", 7)
	if err != nil {
		t.Fatalf("CheckFile() error: %v", err)
	}
	if result.Completed || len(result.UploadedChunks) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestUploadService_CheckFile_Completed(t *testing.T) {
	uploadRepo := &fakeUploadRepo{
		findByFileMD5AndUserIDFn: func(fileMD5 string, userID uint) (*model.FileUpload, error) {
			return &model.FileUpload{FileMD5: fileMD5, UserID: userID, Status: 1}, nil
		},
	}
	svc := NewUploadService(uploadRepo, &fakeUploadUserRepo{}, nil, "uploads", nil)

	result, err := svc.CheckFile(context.Background(), "md5-y", 8)
	if err != nil {
		t.Fatalf("CheckFile() error: %v", err)
	}
	if !result.Completed || len(result.UploadedChunks) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestUploadService_CheckFile_InProgress(t *testing.T) {
	var receivedTotalChunks int
	uploadRepo := &fakeUploadRepo{
		findByFileMD5AndUserIDFn: func(fileMD5 string, userID uint) (*model.FileUpload, error) {
			return &model.FileUpload{
				FileMD5:   fileMD5,
				UserID:    userID,
				Status:    0,
				TotalSize: DefaultChunkSize*2 + 100,
			}, nil
		},
		getUploadedChunksFromRedisFn: func(ctx context.Context, fileMD5 string, userID uint, totalChunks int) ([]int, error) {
			receivedTotalChunks = totalChunks
			return []int{0, 2}, nil
		},
	}
	svc := NewUploadService(uploadRepo, &fakeUploadUserRepo{}, nil, "uploads", nil)

	result, err := svc.CheckFile(context.Background(), "md5-z", 9)
	if err != nil {
		t.Fatalf("CheckFile() error: %v", err)
	}
	if result.Completed {
		t.Fatalf("expected incomplete result, got %+v", result)
	}
	if receivedTotalChunks != 3 {
		t.Fatalf("expected totalChunks=3, got %d", receivedTotalChunks)
	}
	if len(result.UploadedChunks) != 2 || result.UploadedChunks[0] != 0 || result.UploadedChunks[1] != 2 {
		t.Fatalf("unexpected uploaded chunks: %+v", result.UploadedChunks)
	}
}

func TestUploadService_GetUploadStatus_InProgress(t *testing.T) {
	uploadRepo := &fakeUploadRepo{
		findByFileMD5AndUserIDFn: func(fileMD5 string, userID uint) (*model.FileUpload, error) {
			return &model.FileUpload{
				FileMD5:   fileMD5,
				UserID:    userID,
				Status:    0,
				TotalSize: DefaultChunkSize*2 + 100,
			}, nil
		},
		getUploadedChunksFromRedisFn: func(ctx context.Context, fileMD5 string, userID uint, totalChunks int) ([]int, error) {
			return []int{0, 2}, nil
		},
	}
	svc := NewUploadService(uploadRepo, &fakeUploadUserRepo{}, nil, "uploads", nil)

	result, err := svc.GetUploadStatus(context.Background(), "md5-z", 9)
	if err != nil {
		t.Fatalf("GetUploadStatus() error: %v", err)
	}
	if result.FileMD5 != "md5-z" || result.Status != 0 || result.Completed {
		t.Fatalf("unexpected status result: %+v", result)
	}
	if len(result.UploadedChunks) != 2 || result.Progress <= 60 || result.Progress >= 70 {
		t.Fatalf("unexpected upload progress result: %+v", result)
	}
}

func TestUploadService_CheckFastUpload_Completed(t *testing.T) {
	uploadRepo := &fakeUploadRepo{
		findByFileMD5AndUserIDFn: func(fileMD5 string, userID uint) (*model.FileUpload, error) {
			return &model.FileUpload{
				FileMD5:   fileMD5,
				FileName:  "a.pdf",
				TotalSize: 128,
				Status:    1,
				UserID:    userID,
			}, nil
		},
	}
	svc := NewUploadService(uploadRepo, &fakeUploadUserRepo{}, nil, "uploads", nil)

	result, err := svc.CheckFastUpload(context.Background(), "md5-q", 7)
	if err != nil {
		t.Fatalf("CheckFastUpload() error: %v", err)
	}
	if !result.CanQuickUpload || result.FileName != "a.pdf" || result.TotalSize != 128 {
		t.Fatalf("unexpected fast upload result: %+v", result)
	}
}

func TestUploadService_GetSupportedTypes_Sorted(t *testing.T) {
	svc := NewUploadService(&fakeUploadRepo{}, &fakeUploadUserRepo{}, nil, "uploads", nil)

	types := svc.GetSupportedTypes()
	if len(types) == 0 {
		t.Fatal("expected supported types")
	}
	for i := 1; i < len(types); i++ {
		if types[i-1] > types[i] {
			t.Fatalf("expected sorted types, got %v", types)
		}
	}
}

func TestUploadService_UploadChunk_UnsupportedFileType(t *testing.T) {
	svc := NewUploadService(&fakeUploadRepo{}, &fakeUploadUserRepo{}, nil, "uploads", nil)

	_, err := svc.UploadChunk(
		context.Background(),
		"md5-v", "a.exe", 100, 0,
		strings.NewReader("chunk"), int64(len("chunk")),
		1, "team-a", false,
	)
	if !errors.Is(err, ErrUnsupportedFileType) {
		t.Fatalf("expected ErrUnsupportedFileType, got %v", err)
	}
}

func TestUploadService_UploadChunk_AlreadyCompleted(t *testing.T) {
	uploadRepo := &fakeUploadRepo{
		findByFileMD5AndUserIDFn: func(fileMD5 string, userID uint) (*model.FileUpload, error) {
			return &model.FileUpload{
				FileMD5:   fileMD5,
				FileName:  "a.pdf",
				TotalSize: DefaultChunkSize*2 + 1,
				Status:    1,
				UserID:    userID,
			}, nil
		},
	}
	svc := NewUploadService(uploadRepo, &fakeUploadUserRepo{}, nil, "uploads", nil)

	result, err := svc.UploadChunk(
		context.Background(),
		"md5-v", "a.pdf", DefaultChunkSize*2+1, 0,
		strings.NewReader("chunk"), int64(len("chunk")),
		1, "team-a", false,
	)
	if err != nil {
		t.Fatalf("UploadChunk() error: %v", err)
	}
	if result.Progress != 100 {
		t.Fatalf("expected progress 100, got %v", result.Progress)
	}
	if len(result.UploadedChunks) != 3 {
		t.Fatalf("expected 3 chunks, got %+v", result.UploadedChunks)
	}
}

func TestUploadService_UploadChunk_FillOrgTagUserError(t *testing.T) {
	userRepo := &fakeUploadUserRepo{
		findByIDFn: func(userID uint) (*model.User, error) {
			return nil, errors.New("db down")
		},
	}
	svc := NewUploadService(&fakeUploadRepo{}, userRepo, nil, "uploads", nil)

	_, err := svc.UploadChunk(
		context.Background(),
		"md5-v", "a.pdf", 100, 0,
		strings.NewReader("chunk"), int64(len("chunk")),
		1, "", false,
	)
	if !errors.Is(err, ErrInternal) {
		t.Fatalf("expected ErrInternal, got %v", err)
	}
}

func TestUploadService_MergeChunks_NotFound(t *testing.T) {
	uploadRepo := &fakeUploadRepo{
		findByFileMD5AndUserIDFn: func(fileMD5 string, userID uint) (*model.FileUpload, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := NewUploadService(uploadRepo, &fakeUploadUserRepo{}, nil, "uploads", nil)

	_, err := svc.MergeChunks(context.Background(), "md5-1", "a.pdf", 1)
	if !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("expected ErrFileNotFound, got %v", err)
	}
}

func TestUploadService_MergeChunks_AlreadyMerged(t *testing.T) {
	uploadRepo := &fakeUploadRepo{
		findByFileMD5AndUserIDFn: func(fileMD5 string, userID uint) (*model.FileUpload, error) {
			return &model.FileUpload{
				FileMD5:  fileMD5,
				FileName: "a.pdf",
				Status:   1,
				UserID:   userID,
			}, nil
		},
	}
	svc := NewUploadService(uploadRepo, &fakeUploadUserRepo{}, nil, "uploads", nil)

	result, err := svc.MergeChunks(context.Background(), "md5-2", "a.pdf", 11)
	if err != nil {
		t.Fatalf("MergeChunks() error: %v", err)
	}
	if result.ObjectURL != "uploads/11/md5-2/a.pdf" {
		t.Fatalf("unexpected object url: %s", result.ObjectURL)
	}
}

func TestUploadService_MergeChunks_Incomplete(t *testing.T) {
	uploadRepo := &fakeUploadRepo{
		findByFileMD5AndUserIDFn: func(fileMD5 string, userID uint) (*model.FileUpload, error) {
			return &model.FileUpload{
				FileMD5:   fileMD5,
				FileName:  "a.pdf",
				Status:    0,
				TotalSize: DefaultChunkSize*2 + 1, // totalChunks=3
				UserID:    userID,
			}, nil
		},
		getUploadedChunksFromRedisFn: func(ctx context.Context, fileMD5 string, userID uint, totalChunks int) ([]int, error) {
			return []int{0, 1}, nil
		},
	}
	svc := NewUploadService(uploadRepo, &fakeUploadUserRepo{}, nil, "uploads", nil)

	_, err := svc.MergeChunks(context.Background(), "md5-3", "a.pdf", 12)
	if !errors.Is(err, ErrChunksIncomplete) {
		t.Fatalf("expected ErrChunksIncomplete, got %v", err)
	}
}

func TestUploadService_UploadChunk_AlreadyUploadedIdempotentPath(t *testing.T) {
	uploadRepo := &fakeUploadRepo{
		findByFileMD5AndUserIDFn: func(fileMD5 string, userID uint) (*model.FileUpload, error) {
			return &model.FileUpload{
				FileMD5:   fileMD5,
				FileName:  "a.pdf",
				Status:    0,
				TotalSize: DefaultChunkSize * 2,
				UserID:    userID,
			}, nil
		},
		isChunkUploadedFn: func(ctx context.Context, fileMD5 string, userID uint, chunkIndex int) (bool, error) {
			return true, nil
		},
		getUploadedChunksFromRedisFn: func(ctx context.Context, fileMD5 string, userID uint, totalChunks int) ([]int, error) {
			return []int{0, 1}, nil
		},
	}
	svc := NewUploadService(uploadRepo, &fakeUploadUserRepo{}, nil, "uploads", nil)

	result, err := svc.UploadChunk(
		context.Background(),
		"md5-k", "a.pdf", DefaultChunkSize*2, 1,
		strings.NewReader("chunk"), int64(len("chunk")),
		5, "team-a", false,
	)
	if err != nil {
		t.Fatalf("UploadChunk() error: %v", err)
	}
	if len(result.UploadedChunks) != 2 {
		t.Fatalf("unexpected uploaded chunks: %+v", result.UploadedChunks)
	}
	if result.Progress <= 0 || result.Progress > 100 {
		t.Fatalf("unexpected progress: %v", result.Progress)
	}
}

func TestUploadService_UploadChunk_OrgTagFillSuccess(t *testing.T) {
	calledCreate := false
	uploadRepo := &fakeUploadRepo{
		findByFileMD5AndUserIDFn: func(fileMD5 string, userID uint) (*model.FileUpload, error) {
			return nil, gorm.ErrRecordNotFound
		},
		createFn: func(upload *model.FileUpload) error {
			calledCreate = true
			if upload.OrgTag != "team-user" {
				t.Fatalf("expected orgTag filled from user.PrimaryOrg, got %s", upload.OrgTag)
			}
			return errors.New("stop after create")
		},
	}
	userRepo := &fakeUploadUserRepo{
		findByIDFn: func(userID uint) (*model.User, error) {
			return &model.User{ID: userID, PrimaryOrg: "team-user"}, nil
		},
	}
	svc := NewUploadService(uploadRepo, userRepo, nil, "uploads", nil)

	_, err := svc.UploadChunk(
		context.Background(),
		"md5-org", "a.pdf", 100, 0,
		strings.NewReader("chunk"), int64(len("chunk")),
		6, "", true,
	)
	if !calledCreate {
		t.Fatalf("expected Create() to be called")
	}
	if !errors.Is(err, ErrInternal) {
		t.Fatalf("expected ErrInternal after forced create error, got %v", err)
	}
}

func TestUploadService_CheckFile_RepoError(t *testing.T) {
	uploadRepo := &fakeUploadRepo{
		findByFileMD5AndUserIDFn: func(fileMD5 string, userID uint) (*model.FileUpload, error) {
			return nil, errors.New("db error")
		},
	}
	svc := NewUploadService(uploadRepo, &fakeUploadUserRepo{}, nil, "uploads", nil)

	_, err := svc.CheckFile(context.Background(), "md5-err", 1)
	if !errors.Is(err, ErrInternal) {
		t.Fatalf("expected ErrInternal, got %v", err)
	}
}

func TestUploadService_UploadChunk_NilReaderStillSupportedInIdempotentBranch(t *testing.T) {
	uploadRepo := &fakeUploadRepo{
		findByFileMD5AndUserIDFn: func(fileMD5 string, userID uint) (*model.FileUpload, error) {
			return &model.FileUpload{
				FileMD5:   fileMD5,
				FileName:  "a.pdf",
				Status:    0,
				TotalSize: DefaultChunkSize,
				UserID:    userID,
			}, nil
		},
		isChunkUploadedFn: func(ctx context.Context, fileMD5 string, userID uint, chunkIndex int) (bool, error) {
			return true, nil
		},
	}
	svc := NewUploadService(uploadRepo, &fakeUploadUserRepo{}, nil, "uploads", nil)

	result, err := svc.UploadChunk(
		context.Background(),
		"md5-nil", "a.pdf", DefaultChunkSize, 0,
		io.NopCloser(strings.NewReader("")), 0,
		3, "team-a", false,
	)
	if err != nil {
		t.Fatalf("UploadChunk() error: %v", err)
	}
	if result.Progress < 0 {
		t.Fatalf("unexpected progress: %v", result.Progress)
	}
}

func TestUploadService_ProduceFileTask_Success(t *testing.T) {
	producer := &fakeTaskProducer{}
	svc := &uploadService{
		taskProducer: producer,
	}

	upload := &model.FileUpload{
		FileMD5:  "md5-ok",
		FileName: "a.pdf",
		UserID:   9,
		OrgTag:   "team-a",
		IsPublic: true,
	}
	svc.produceFileTask(context.Background(), upload, "uploads/9/md5-ok/a.pdf")

	if producer.called != 1 {
		t.Fatalf("expected producer called once, got %d", producer.called)
	}
	if producer.lastTask.FileMD5 != "md5-ok" || producer.lastTask.ObjectKey != "uploads/9/md5-ok/a.pdf" {
		t.Fatalf("unexpected produced task: %+v", producer.lastTask)
	}
}

func TestUploadService_ProduceFileTask_FailureShouldNotPanic(t *testing.T) {
	producer := &fakeTaskProducer{
		produceFn: func(ctx context.Context, task tasks.FileProcessingTask) error {
			return errors.New("kafka unavailable")
		},
	}
	svc := &uploadService{
		taskProducer: producer,
	}

	upload := &model.FileUpload{
		FileMD5:  "md5-fail",
		FileName: "b.pdf",
		UserID:   10,
	}
	svc.produceFileTask(context.Background(), upload, "uploads/10/md5-fail/b.pdf")

	if producer.called != 1 {
		t.Fatalf("expected producer called once, got %d", producer.called)
	}
}
