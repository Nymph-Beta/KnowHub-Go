package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"

	"pai_smart_go_v2/internal/config"
	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/pkg/tasks"
)

type fakeEmbeddingClient struct {
	vector []float32
	err    error
}

func (f *fakeEmbeddingClient) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.vector, nil
}

func TestSplitText_LongTextWithOverlap(t *testing.T) {
	text := strings.Repeat("a", 2500)

	chunks, err := splitText(text, 1000, 100)
	if err != nil {
		t.Fatalf("splitText() error: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if len([]rune(chunks[0])) != 1000 || len([]rune(chunks[1])) != 1000 || len([]rune(chunks[2])) != 700 {
		t.Fatalf("unexpected chunk lengths: %d %d %d", len([]rune(chunks[0])), len([]rune(chunks[1])), len([]rune(chunks[2])))
	}
	if chunks[0][900:] != chunks[1][:100] {
		t.Fatalf("expected overlap to match between chunk0 and chunk1")
	}
}

func TestSplitText_ChineseRuneSafe(t *testing.T) {
	chunks, err := splitText("你好世界编程", 3, 1)
	if err != nil {
		t.Fatalf("splitText() error: %v", err)
	}

	want := []string{"你好世", "世界编", "编程"}
	if len(chunks) != len(want) {
		t.Fatalf("unexpected chunk count: got=%d want=%d", len(chunks), len(want))
	}
	for i := range want {
		if chunks[i] != want[i] {
			t.Fatalf("chunk[%d]=%q, want=%q", i, chunks[i], want[i])
		}
	}
}

func TestSplitText_ShortText(t *testing.T) {
	chunks, err := splitText("hello", 1000, 100)
	if err != nil {
		t.Fatalf("splitText() error: %v", err)
	}
	if len(chunks) != 1 || chunks[0] != "hello" {
		t.Fatalf("unexpected chunks: %+v", chunks)
	}
}

func TestSplitText_EmptyText(t *testing.T) {
	chunks, err := splitText("", 1000, 100)
	if err != nil {
		t.Fatalf("splitText() error: %v", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("expected empty chunks, got %+v", chunks)
	}
}

func TestSplitText_InvalidConfig(t *testing.T) {
	if _, err := splitText("abc", 100, 100); err == nil {
		t.Fatalf("expected invalid config error")
	}
}

func TestBuildDocumentVectors(t *testing.T) {
	task := tasks.FileProcessingTask{
		FileMD5:  "md5v",
		UserID:   9,
		OrgTag:   "team-a",
		IsPublic: true,
	}

	vectors := buildDocumentVectors(task, []string{"first", "second"}, "text-embedding-v4")
	if len(vectors) != 2 {
		t.Fatalf("unexpected vector count: %d", len(vectors))
	}
	if vectors[0].ChunkID != 0 || vectors[1].ChunkID != 1 {
		t.Fatalf("unexpected chunk ids: %+v", vectors)
	}
	if vectors[0].FileMD5 != "md5v" || vectors[0].OrgTag != "team-a" || !vectors[0].IsPublic {
		t.Fatalf("unexpected vector metadata: %+v", vectors[0])
	}
	if vectors[0].ModelVersion != "text-embedding-v4" {
		t.Fatalf("unexpected model version: %+v", vectors[0])
	}
}

func TestBuildEsDocument(t *testing.T) {
	doc := buildEsDocument(model.DocumentVector{
		FileMD5:      "md5v",
		ChunkID:      2,
		TextContent:  "chunk text",
		ModelVersion: "text-embedding-v4",
		UserID:       9,
		OrgTag:       "team-a",
		IsPublic:     true,
	}, []float32{0.1, 0.2}, "fallback-model")

	if doc.VectorID != "md5v_2" {
		t.Fatalf("unexpected vector id: %+v", doc)
	}
	if doc.ModelVersion != "text-embedding-v4" || len(doc.Vector) != 2 {
		t.Fatalf("unexpected es document: %+v", doc)
	}
}

func TestProcessor_VectorizeDocuments(t *testing.T) {
	p := &Processor{
		embedding: &fakeEmbeddingClient{vector: []float32{0.1, 0.2}},
		embeddingCfg: config.EmbeddingConfig{
			Model:      "text-embedding-v4",
			Dimensions: 2,
		},
	}

	docs, dims, err := p.vectorizeDocuments(context.Background(), []model.DocumentVector{
		{
			FileMD5:      "md5v",
			ChunkID:      0,
			TextContent:  "first",
			ModelVersion: "text-embedding-v4",
			UserID:       1,
			OrgTag:       "team-a",
			IsPublic:     true,
		},
	})
	if err != nil {
		t.Fatalf("vectorizeDocuments() error = %v", err)
	}
	if dims != 2 || len(docs) != 1 {
		t.Fatalf("unexpected vectorize result: dims=%d docs=%d", dims, len(docs))
	}
	if docs[0].VectorID != "md5v_0" {
		t.Fatalf("unexpected es doc: %+v", docs[0])
	}
}

func TestProcessor_VectorizeDocuments_Error(t *testing.T) {
	p := &Processor{
		embedding: &fakeEmbeddingClient{err: errors.New("dashscope failed")},
		embeddingCfg: config.EmbeddingConfig{
			Model:      "text-embedding-v4",
			Dimensions: 2,
		},
	}

	if _, _, err := p.vectorizeDocuments(context.Background(), []model.DocumentVector{
		{FileMD5: "md5v", ChunkID: 0, TextContent: "first"},
	}); err == nil {
		t.Fatalf("expected vectorizeDocuments() error")
	}
}
