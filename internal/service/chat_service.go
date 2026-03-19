package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"text/template"
	"time"

	"pai_smart_go_v2/internal/config"
	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/pkg/llm"
	"pai_smart_go_v2/pkg/log"
)

const defaultChatSearchTopK = 6

type chatSearchProvider interface {
	HybridSearch(ctx context.Context, query string, topK int, user *model.User) ([]model.SearchResponseDTO, error)
}

type chatConversationRepository interface {
	GetOrCreateConversationID(ctx context.Context, userID uint) (string, error)
	GetConversationHistory(ctx context.Context, conversationID string) ([]model.ChatMessage, error)
	UpdateConversationHistory(ctx context.Context, conversationID string, messages []model.ChatMessage) error
}

type ChatResponseWriter interface {
	WriteJSON(v interface{}) error
}

type ChatService interface {
	StreamResponse(ctx context.Context, question string, user *model.User, writer ChatResponseWriter, shouldStop func() bool) error
}

type chatService struct {
	searchService    chatSearchProvider
	llmClient        llm.Client
	conversationRepo chatConversationRepository
	llmCfg           config.LLMConfig
}

type wsWriterInterceptor struct {
	writer     ChatResponseWriter
	shouldStop func() bool
	builder    strings.Builder
}

func NewChatService(
	searchService chatSearchProvider,
	llmClient llm.Client,
	conversationRepo chatConversationRepository,
	llmCfg config.LLMConfig,
) ChatService {
	return &chatService{
		searchService:    searchService,
		llmClient:        llmClient,
		conversationRepo: conversationRepo,
		llmCfg:           llmCfg,
	}
}

func (s *chatService) StreamResponse(ctx context.Context, question string, user *model.User, writer ChatResponseWriter, shouldStop func() bool) error {
	if s.searchService == nil || s.llmClient == nil || s.conversationRepo == nil || writer == nil {
		return ErrInternal
	}
	if user == nil || strings.TrimSpace(question) == "" {
		return ErrInvalidInput
	}

	question = strings.TrimSpace(question)
	startedAt := time.Now()
	conversationID, err := s.conversationRepo.GetOrCreateConversationID(ctx, user.ID)
	if err != nil {
		log.Errorf("StreamResponse: get conversation id failed: %v", err)
		return ErrInternal
	}
	log.Infow("chat stream started",
		"user_id", user.ID,
		"conversation_id", conversationID,
		"provider", s.providerName(),
		"api_style", s.apiStyle(),
		"model", s.llmCfg.Model,
		"question_preview", truncateForLog(question, 120),
		"question_len", len([]rune(question)),
	)

	history, err := s.conversationRepo.GetConversationHistory(ctx, conversationID)
	if err != nil {
		log.Errorf("StreamResponse: get conversation history failed: %v", err)
		return ErrInternal
	}

	searchResults, err := s.searchService.HybridSearch(ctx, question, defaultChatSearchTopK, user)
	if err != nil {
		return err
	}
	log.Infow("chat retrieval completed",
		"user_id", user.ID,
		"conversation_id", conversationID,
		"hits", len(searchResults),
		"top_k", defaultChatSearchTopK,
	)

	assistantAnswer := strings.TrimSpace(s.llmCfg.Prompt.NoResultText)
	if len(searchResults) == 0 {
		if assistantAnswer == "" {
			assistantAnswer = "当前知识库里没有检索到足够相关的资料，暂时无法给出可靠回答。"
		}
		if err := writer.WriteJSON(map[string]string{"chunk": assistantAnswer}); err != nil {
			return err
		}
		if err := writer.WriteJSON(map[string]string{"type": "completion", "status": "finished"}); err != nil {
			return err
		}
		s.persistConversation(conversationID, history, question, assistantAnswer)
		log.Infow("chat stream finished",
			"user_id", user.ID,
			"conversation_id", conversationID,
			"status", "finished",
			"hits", 0,
			"answer_len", len([]rune(assistantAnswer)),
			"latency_ms", time.Since(startedAt).Milliseconds(),
		)
		return nil
	}

	interceptor := &wsWriterInterceptor{
		writer:     writer,
		shouldStop: shouldStop,
	}

	messages := append([]llm.Message{{Role: "system", Content: s.buildSystemPrompt(searchResults)}}, toLLMMessages(history)...)
	messages = append(messages, llm.Message{Role: "user", Content: question})

	err = s.llmClient.StreamChat(ctx, messages, interceptor)
	status := "finished"
	if err != nil {
		if errors.Is(err, context.Canceled) || (shouldStop != nil && shouldStop()) {
			status = "stopped"
		} else {
			log.Errorf("StreamResponse: llm stream failed: %v", err)
			return ErrInternal
		}
	}

	if writeErr := writer.WriteJSON(map[string]string{"type": "completion", "status": status}); writeErr != nil {
		return writeErr
	}

	answer := strings.TrimSpace(interceptor.builder.String())
	if answer != "" {
		s.persistConversation(conversationID, history, question, answer)
	}
	log.Infow("chat stream finished",
		"user_id", user.ID,
		"conversation_id", conversationID,
		"status", status,
		"hits", len(searchResults),
		"answer_len", len([]rune(answer)),
		"latency_ms", time.Since(startedAt).Milliseconds(),
	)
	return nil
}

func (s *chatService) buildSystemPrompt(results []model.SearchResponseDTO) string {
	templateContent := strings.TrimSpace(s.llmCfg.Prompt.Template)
	if templateContent != "" {
		rendered, err := renderPromptTemplate(templateContent, results, s.llmCfg.Prompt.RefStart, s.llmCfg.Prompt.RefEnd)
		if err == nil {
			return rendered
		}
		log.Warnf("buildSystemPrompt: render prompt template failed, fallback to inline rules: %v", err)
	}

	rules := strings.TrimSpace(s.llmCfg.Prompt.Rules)
	if rules == "" {
		rules = "你是 PaiSmart 的企业知识库助手。请严格基于参考资料回答；如果参考资料不足，请明确说明不知道，不要编造。"
	}
	refStart := strings.TrimSpace(s.llmCfg.Prompt.RefStart)
	if refStart == "" {
		refStart = "<<REF>>"
	}
	refEnd := strings.TrimSpace(s.llmCfg.Prompt.RefEnd)
	if refEnd == "" {
		refEnd = "<<END>>"
	}

	var builder strings.Builder
	builder.WriteString(rules)
	builder.WriteString("\n\n")
	builder.WriteString(refStart)
	builder.WriteByte('\n')
	for i, item := range results {
		fileName := strings.TrimSpace(item.FileName)
		if fileName == "" {
			fileName = item.FileMD5
		}
		builder.WriteString(fmt.Sprintf("[%d] (%s) %s\n", i+1, fileName, strings.TrimSpace(item.TextContent)))
	}
	builder.WriteString(refEnd)
	return builder.String()
}

func renderPromptTemplate(templateContent string, results []model.SearchResponseDTO, refStart, refEnd string) (string, error) {
	if strings.TrimSpace(refStart) == "" {
		refStart = "<<REF>>"
	}
	if strings.TrimSpace(refEnd) == "" {
		refEnd = "<<END>>"
	}

	tpl, err := template.New("chat-system-prompt").Parse(templateContent)
	if err != nil {
		return "", err
	}

	var references strings.Builder
	for i, item := range results {
		fileName := strings.TrimSpace(item.FileName)
		if fileName == "" {
			fileName = item.FileMD5
		}
		references.WriteString(fmt.Sprintf("[%d] (%s) %s\n", i+1, fileName, strings.TrimSpace(item.TextContent)))
	}

	var rendered strings.Builder
	err = tpl.Execute(&rendered, map[string]string{
		"References": references.String(),
		"RefStart":   refStart,
		"RefEnd":     refEnd,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(rendered.String()), nil
}

func (s *chatService) persistConversation(conversationID string, history []model.ChatMessage, question, answer string) {
	if strings.TrimSpace(conversationID) == "" || strings.TrimSpace(answer) == "" {
		return
	}

	nextHistory := append(append([]model.ChatMessage{}, history...),
		model.ChatMessage{Role: "user", Content: question, CreatedAt: time.Now()},
		model.ChatMessage{Role: "assistant", Content: answer, CreatedAt: time.Now()},
	)

	if err := s.conversationRepo.UpdateConversationHistory(context.Background(), conversationID, nextHistory); err != nil {
		log.Errorf("persistConversation: save conversation history failed: %v", err)
	}
}

func toLLMMessages(history []model.ChatMessage) []llm.Message {
	if len(history) == 0 {
		return []llm.Message{}
	}

	result := make([]llm.Message, 0, len(history))
	for _, msg := range history {
		role := strings.TrimSpace(msg.Role)
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		switch role {
		case "user", "assistant":
			result = append(result, llm.Message{Role: role, Content: content})
		}
	}
	return result
}

func (w *wsWriterInterceptor) WriteMessage(_ int, data []byte) error {
	if w.shouldStop != nil && w.shouldStop() {
		return context.Canceled
	}

	chunk := string(data)
	w.builder.WriteString(chunk)
	return w.writer.WriteJSON(map[string]string{"chunk": chunk})
}

func (s *chatService) providerName() string {
	provider := strings.TrimSpace(s.llmCfg.Provider)
	if provider == "" {
		return "unknown"
	}
	return provider
}

func (s *chatService) apiStyle() string {
	apiStyle := strings.TrimSpace(s.llmCfg.APIStyle)
	if apiStyle == "" {
		return "openai_compatible"
	}
	return apiStyle
}

func truncateForLog(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}
