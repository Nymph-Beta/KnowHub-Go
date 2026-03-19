package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"pai_smart_go_v2/internal/config"
	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/pkg/llm"
)

type fakeChatSearchService struct {
	results []model.SearchResponseDTO
	err     error
	query   string
	topK    int
	userID  uint
}

func (f *fakeChatSearchService) HybridSearch(ctx context.Context, query string, topK int, user *model.User) ([]model.SearchResponseDTO, error) {
	f.query = query
	f.topK = topK
	if user != nil {
		f.userID = user.ID
	}
	return f.results, f.err
}

type fakeConversationRepo struct {
	conversationID string
	history        []model.ChatMessage
	savedHistory   []model.ChatMessage
}

func (f *fakeConversationRepo) GetOrCreateConversationID(ctx context.Context, userID uint) (string, error) {
	if f.conversationID == "" {
		f.conversationID = "conv-1"
	}
	return f.conversationID, nil
}

func (f *fakeConversationRepo) GetConversationHistory(ctx context.Context, conversationID string) ([]model.ChatMessage, error) {
	return append([]model.ChatMessage{}, f.history...), nil
}

func (f *fakeConversationRepo) UpdateConversationHistory(ctx context.Context, conversationID string, messages []model.ChatMessage) error {
	f.savedHistory = append([]model.ChatMessage{}, messages...)
	return nil
}

type fakeLLMClient struct {
	streamChatFn func(ctx context.Context, messages []llm.Message, writer llm.MessageWriter) error
}

func (f *fakeLLMClient) StreamChat(ctx context.Context, messages []llm.Message, writer llm.MessageWriter) error {
	if f.streamChatFn != nil {
		return f.streamChatFn(ctx, messages, writer)
	}
	return nil
}

type fakeChatWriter struct {
	payloads []map[string]string
}

func (w *fakeChatWriter) WriteJSON(v interface{}) error {
	payload, ok := v.(map[string]string)
	if !ok {
		return errors.New("unexpected payload type")
	}
	w.payloads = append(w.payloads, payload)
	return nil
}

func TestChatServiceStreamResponseSuccess(t *testing.T) {
	searchSvc := &fakeChatSearchService{
		results: []model.SearchResponseDTO{{
			FileMD5:     "md5",
			FileName:    "go.pdf",
			TextContent: "Go 使用 goroutine 实现并发。",
		}},
	}
	conversationRepo := &fakeConversationRepo{
		history: []model.ChatMessage{{Role: "user", Content: "上一轮问题"}},
	}
	var gotMessages []llm.Message
	llmClient := &fakeLLMClient{
		streamChatFn: func(ctx context.Context, messages []llm.Message, writer llm.MessageWriter) error {
			gotMessages = append([]llm.Message{}, messages...)
			if err := writer.WriteMessage(llm.TextMessageType, []byte("Go")); err != nil {
				return err
			}
			return writer.WriteMessage(llm.TextMessageType, []byte(" 很适合高并发场景"))
		},
	}

	svc := NewChatService(searchSvc, llmClient, conversationRepo, config.LLMConfig{
		Provider: "deepseek",
		APIStyle: "openai_compatible",
		Prompt: config.LLMPromptConfig{
			Template: "请基于资料回答\n\n{{.RefStart}}\n{{.References}}{{.RefEnd}}",
			RefStart: "<<REF>>",
			RefEnd:   "<<END>>",
		},
	})

	writer := &fakeChatWriter{}
	err := svc.StreamResponse(context.Background(), "Go 有什么特点？", &model.User{ID: 9}, writer, func() bool { return false })
	if err != nil {
		t.Fatalf("StreamResponse() error = %v", err)
	}

	if searchSvc.query != "Go 有什么特点？" || searchSvc.topK != defaultChatSearchTopK || searchSvc.userID != 9 {
		t.Fatalf("unexpected search input: query=%q topK=%d userID=%d", searchSvc.query, searchSvc.topK, searchSvc.userID)
	}
	if len(gotMessages) != 3 {
		t.Fatalf("expected 3 llm messages, got %d", len(gotMessages))
	}
	if !strings.Contains(gotMessages[0].Content, "<<REF>>") || !strings.Contains(gotMessages[0].Content, "go.pdf") {
		t.Fatalf("unexpected system prompt: %s", gotMessages[0].Content)
	}
	if len(writer.payloads) != 3 {
		t.Fatalf("unexpected payload count: %d", len(writer.payloads))
	}
	if writer.payloads[0]["chunk"] != "Go" || writer.payloads[1]["chunk"] != " 很适合高并发场景" {
		t.Fatalf("unexpected chunks: %+v", writer.payloads)
	}
	if writer.payloads[2]["status"] != "finished" {
		t.Fatalf("unexpected completion payload: %+v", writer.payloads[2])
	}
	if len(conversationRepo.savedHistory) != 3 {
		t.Fatalf("unexpected saved history length: %d", len(conversationRepo.savedHistory))
	}
	if conversationRepo.savedHistory[2].Content != "Go 很适合高并发场景" {
		t.Fatalf("unexpected saved answer: %s", conversationRepo.savedHistory[2].Content)
	}
}

func TestChatServiceStreamResponseNoSearchResult(t *testing.T) {
	conversationRepo := &fakeConversationRepo{}
	svc := NewChatService(&fakeChatSearchService{}, &fakeLLMClient{}, conversationRepo, config.LLMConfig{
		Prompt: config.LLMPromptConfig{
			NoResultText: "没有命中资料",
		},
	})

	writer := &fakeChatWriter{}
	err := svc.StreamResponse(context.Background(), "问题", &model.User{ID: 1}, writer, func() bool { return false })
	if err != nil {
		t.Fatalf("StreamResponse() error = %v", err)
	}

	if len(writer.payloads) != 2 {
		t.Fatalf("unexpected payload count: %d", len(writer.payloads))
	}
	if writer.payloads[0]["chunk"] != "没有命中资料" {
		t.Fatalf("unexpected no result payload: %+v", writer.payloads[0])
	}
	if len(conversationRepo.savedHistory) != 2 {
		t.Fatalf("unexpected saved history: %+v", conversationRepo.savedHistory)
	}
}

func TestChatServiceStreamResponseStopped(t *testing.T) {
	conversationRepo := &fakeConversationRepo{}
	llmClient := &fakeLLMClient{
		streamChatFn: func(ctx context.Context, messages []llm.Message, writer llm.MessageWriter) error {
			if err := writer.WriteMessage(llm.TextMessageType, []byte("partial")); err != nil {
				return err
			}
			return context.Canceled
		},
	}
	svc := NewChatService(&fakeChatSearchService{
		results: []model.SearchResponseDTO{{FileName: "doc.txt", TextContent: "chunk"}},
	}, llmClient, conversationRepo, config.LLMConfig{})

	writer := &fakeChatWriter{}
	err := svc.StreamResponse(context.Background(), "问题", &model.User{ID: 1}, writer, func() bool { return false })
	if err != nil {
		t.Fatalf("StreamResponse() error = %v", err)
	}

	if got := writer.payloads[len(writer.payloads)-1]["status"]; got != "stopped" {
		t.Fatalf("expected stopped status, got %q", got)
	}
	if len(conversationRepo.savedHistory) != 2 || conversationRepo.savedHistory[1].Content != "partial" {
		t.Fatalf("unexpected saved history after stop: %+v", conversationRepo.savedHistory)
	}
}
