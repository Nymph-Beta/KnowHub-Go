package service

import (
	"context"
	"testing"
	"time"

	"pai_smart_go_v2/internal/model"
)

type fakeConversationRepository struct {
	getConversationIDFn             func(ctx context.Context, userID uint) (string, error)
	getOrCreateConversationIDFn     func(ctx context.Context, userID uint) (string, error)
	getConversationHistoryFn        func(ctx context.Context, conversationID string) ([]model.ChatMessage, error)
	updateConversationHistoryFn     func(ctx context.Context, conversationID string, messages []model.ChatMessage) error
	getAllUserConversationMappingsFn func(ctx context.Context) (map[uint]string, error)
}

func (f *fakeConversationRepository) GetConversationID(ctx context.Context, userID uint) (string, error) {
	if f.getConversationIDFn != nil {
		return f.getConversationIDFn(ctx, userID)
	}
	return "", nil
}

func (f *fakeConversationRepository) GetOrCreateConversationID(ctx context.Context, userID uint) (string, error) {
	if f.getOrCreateConversationIDFn != nil {
		return f.getOrCreateConversationIDFn(ctx, userID)
	}
	return "", nil
}

func (f *fakeConversationRepository) GetConversationHistory(ctx context.Context, conversationID string) ([]model.ChatMessage, error) {
	if f.getConversationHistoryFn != nil {
		return f.getConversationHistoryFn(ctx, conversationID)
	}
	return []model.ChatMessage{}, nil
}

func (f *fakeConversationRepository) UpdateConversationHistory(ctx context.Context, conversationID string, messages []model.ChatMessage) error {
	if f.updateConversationHistoryFn != nil {
		return f.updateConversationHistoryFn(ctx, conversationID, messages)
	}
	return nil
}

func (f *fakeConversationRepository) GetAllUserConversationMappings(ctx context.Context) (map[uint]string, error) {
	if f.getAllUserConversationMappingsFn != nil {
		return f.getAllUserConversationMappingsFn(ctx)
	}
	return map[uint]string{}, nil
}

type fakeConversationUserFinder struct {
	findByIDFn func(userID uint) (*model.User, error)
}

func (f *fakeConversationUserFinder) FindByID(userID uint) (*model.User, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(userID)
	}
	return &model.User{ID: userID, Username: "user"}, nil
}

func TestConversationService_GetConversationHistory(t *testing.T) {
	now := time.Now()
	svc := NewConversationService(
		&fakeConversationRepository{
			getConversationIDFn: func(ctx context.Context, userID uint) (string, error) {
				return "conv-1", nil
			},
			getConversationHistoryFn: func(ctx context.Context, conversationID string) ([]model.ChatMessage, error) {
				return []model.ChatMessage{{Role: "user", Content: "hello", CreatedAt: now}}, nil
			},
		},
		&fakeConversationUserFinder{},
	)

	history, err := svc.GetConversationHistory(context.Background(), 3)
	if err != nil {
		t.Fatalf("GetConversationHistory() error = %v", err)
	}
	if len(history) != 1 || history[0].Content != "hello" {
		t.Fatalf("unexpected history: %+v", history)
	}
}

func TestConversationService_GetAllConversations_Filter(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)
	inRange := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	outOfRange := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)

	svc := NewConversationService(
		&fakeConversationRepository{
			getConversationIDFn: func(ctx context.Context, userID uint) (string, error) {
				return "conv-9", nil
			},
			getConversationHistoryFn: func(ctx context.Context, conversationID string) ([]model.ChatMessage, error) {
				return []model.ChatMessage{
					{Role: "user", Content: "keep", CreatedAt: inRange},
					{Role: "assistant", Content: "drop", CreatedAt: outOfRange},
				}, nil
			},
		},
		&fakeConversationUserFinder{
			findByIDFn: func(userID uint) (*model.User, error) {
				return &model.User{ID: userID, Username: "alice"}, nil
			},
		},
	)

	userID := uint(9)
	records, err := svc.GetAllConversations(context.Background(), ConversationAdminFilter{
		UserID:    &userID,
		StartTime: &start,
		EndTime:   &end,
	})
	if err != nil {
		t.Fatalf("GetAllConversations() error = %v", err)
	}
	if len(records) != 1 || records[0].Content != "keep" || records[0].Username != "alice" {
		t.Fatalf("unexpected records: %+v", records)
	}
}
