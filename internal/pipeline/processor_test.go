package pipeline

import (
	"strings"
	"testing"

	"pai_smart_go_v2/pkg/tasks"
)

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

	vectors := buildDocumentVectors(task, []string{"first", "second"})
	if len(vectors) != 2 {
		t.Fatalf("unexpected vector count: %d", len(vectors))
	}
	if vectors[0].ChunkID != 0 || vectors[1].ChunkID != 1 {
		t.Fatalf("unexpected chunk ids: %+v", vectors)
	}
	if vectors[0].FileMD5 != "md5v" || vectors[0].OrgTag != "team-a" || !vectors[0].IsPublic {
		t.Fatalf("unexpected vector metadata: %+v", vectors[0])
	}
}
