package service

import (
	"context"
	"errors"
	"testing"

	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/pkg/es"
)

type fakeSearchEmbeddingClient struct {
	createEmbeddingFn func(ctx context.Context, text string) ([]float32, error)
}

func (f *fakeSearchEmbeddingClient) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	if f.createEmbeddingFn != nil {
		return f.createEmbeddingFn(ctx, text)
	}
	return nil, nil
}

type fakeSearchESClient struct {
	searchDocumentsFn func(ctx context.Context, req es.SearchRequest) ([]es.SearchHit, error)
}

func (f *fakeSearchESClient) EnsureIndex(ctx context.Context) error {
	return nil
}

func (f *fakeSearchESClient) BulkIndexDocuments(ctx context.Context, docs []model.EsDocument) error {
	return nil
}

func (f *fakeSearchESClient) SearchDocuments(ctx context.Context, req es.SearchRequest) ([]es.SearchHit, error) {
	if f.searchDocumentsFn != nil {
		return f.searchDocumentsFn(ctx, req)
	}
	return nil, nil
}

func (f *fakeSearchESClient) DeleteDocumentsByFileMD5(ctx context.Context, fileMD5 string) error {
	return nil
}

func (f *fakeSearchESClient) IndexName() string {
	return "knowledge_base"
}

type fakeSearchUserOrgTagProvider struct {
	getUserEffectiveOrgTagsFn func(userID uint) ([]model.OrganizationTag, error)
}

func (f *fakeSearchUserOrgTagProvider) GetUserEffectiveOrgTags(userID uint) ([]model.OrganizationTag, error) {
	if f.getUserEffectiveOrgTagsFn != nil {
		return f.getUserEffectiveOrgTagsFn(userID)
	}
	return nil, nil
}

type fakeSearchUploadRepository struct {
	findBatchByMD5sFn func(fileMD5s []string) ([]model.FileUpload, error)
}

func (f *fakeSearchUploadRepository) FindBatchByMD5s(fileMD5s []string) ([]model.FileUpload, error) {
	if f.findBatchByMD5sFn != nil {
		return f.findBatchByMD5sFn(fileMD5s)
	}
	return nil, nil
}

func TestSearchService_HybridSearch_Success(t *testing.T) {
	svc := NewSearchService(
		&fakeSearchEmbeddingClient{
			createEmbeddingFn: func(ctx context.Context, text string) ([]float32, error) {
				if text != "请问 Go 并发是什么？" {
					t.Fatalf("unexpected raw query: %s", text)
				}
				return []float32{0.1, 0.2}, nil
			},
		},
		&fakeSearchESClient{
			searchDocumentsFn: func(ctx context.Context, req es.SearchRequest) ([]es.SearchHit, error) {
				if req.Query != "go 并发" {
					t.Fatalf("unexpected normalized query: %s", req.Query)
				}
				if req.Phrase != "go 并发" {
					t.Fatalf("unexpected phrase query: %s", req.Phrase)
				}
				if req.TopK != 5 || req.KNNK != 150 || req.NumCandidates != 300 {
					t.Fatalf("unexpected retrieval params: %+v", req)
				}
				if len(req.OrgTags) != 2 || req.OrgTags[0] != "dept:tech" || req.OrgTags[1] != "dept:tech:backend" {
					t.Fatalf("unexpected org tags: %+v", req.OrgTags)
				}
				return []es.SearchHit{
					{
						Score: 7.2,
						Source: model.EsDocument{
							FileMD5:     "md5-a",
							ChunkID:     2,
							TextContent: "Go 的并发模型基于 goroutine",
							UserID:      8,
							OrgTag:      "dept:tech",
							IsPublic:    false,
						},
					},
				}, nil
			},
		},
		&fakeSearchUserOrgTagProvider{
			getUserEffectiveOrgTagsFn: func(userID uint) ([]model.OrganizationTag, error) {
				if userID != 8 {
					t.Fatalf("unexpected userID: %d", userID)
				}
				return []model.OrganizationTag{
					{TagID: "dept:tech"},
					{TagID: "dept:tech:backend"},
				}, nil
			},
		},
		&fakeSearchUploadRepository{
			findBatchByMD5sFn: func(fileMD5s []string) ([]model.FileUpload, error) {
				if len(fileMD5s) != 1 || fileMD5s[0] != "md5-a" {
					t.Fatalf("unexpected md5 list: %+v", fileMD5s)
				}
				return []model.FileUpload{{FileMD5: "md5-a", FileName: "go.pdf"}}, nil
			},
		},
	)

	results, err := svc.HybridSearch(context.Background(), "请问 Go 并发是什么？", 5, &model.User{ID: 8})
	if err != nil {
		t.Fatalf("HybridSearch() error = %v", err)
	}
	if len(results) != 1 || results[0].FileName != "go.pdf" || results[0].Score != 7.2 {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestSearchService_HybridSearch_EmptyQuery(t *testing.T) {
	svc := NewSearchService(
		&fakeSearchEmbeddingClient{},
		&fakeSearchESClient{},
		&fakeSearchUserOrgTagProvider{},
		&fakeSearchUploadRepository{},
	)

	_, err := svc.HybridSearch(context.Background(), "   ", 5, &model.User{ID: 1})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestSearchService_HybridSearch_EmbeddingFailure(t *testing.T) {
	svc := NewSearchService(
		&fakeSearchEmbeddingClient{
			createEmbeddingFn: func(ctx context.Context, text string) ([]float32, error) {
				return nil, errors.New("embedding failed")
			},
		},
		&fakeSearchESClient{},
		&fakeSearchUserOrgTagProvider{
			getUserEffectiveOrgTagsFn: func(userID uint) ([]model.OrganizationTag, error) {
				return []model.OrganizationTag{}, nil
			},
		},
		&fakeSearchUploadRepository{},
	)

	_, err := svc.HybridSearch(context.Background(), "go", 5, &model.User{ID: 1})
	if !errors.Is(err, ErrInternal) {
		t.Fatalf("expected ErrInternal, got %v", err)
	}
}

func TestNormalizeQuery(t *testing.T) {
	normalized, phrase := normalizeQuery("请问，Go 语言是什么？")
	if normalized != "go 语言" {
		t.Fatalf("unexpected normalized query: %q", normalized)
	}
	if phrase != "go 语言" {
		t.Fatalf("unexpected phrase query: %q", phrase)
	}
}

func TestNormalizeTopK(t *testing.T) {
	if got := normalizeTopK(0); got != defaultSearchTopK {
		t.Fatalf("unexpected default topK: %d", got)
	}
	if got := normalizeTopK(999); got != maxSearchTopK {
		t.Fatalf("unexpected capped topK: %d", got)
	}
}

func TestUniqueFileMD5s(t *testing.T) {
	md5s := uniqueFileMD5s([]es.SearchHit{
		{Source: model.EsDocument{FileMD5: "a"}},
		{Source: model.EsDocument{FileMD5: "a"}},
		{Source: model.EsDocument{FileMD5: "b"}},
	})
	if len(md5s) != 2 || md5s[0] != "a" || md5s[1] != "b" {
		t.Fatalf("unexpected md5s: %+v", md5s)
	}
}
