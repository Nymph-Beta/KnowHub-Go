package repository

import (
	"context"
	"testing"
)

func TestConversationRepository_GetConversationID_NotFound(t *testing.T) {
	rdb := newFakeRedisClient(t)
	repo := NewConversationRepository(rdb)

	conversationID, err := repo.GetConversationID(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetConversationID() error = %v", err)
	}
	if conversationID != "" {
		t.Fatalf("expected empty conversationID, got %q", conversationID)
	}
}

func TestConversationRepository_GetAllUserConversationMappings(t *testing.T) {
	rdb := newFakeRedisClient(t)
	repo := NewConversationRepository(rdb)

	if err := rdb.Set(context.Background(), "user:3:current_conversation", "conv-3", 0).Err(); err != nil {
		t.Fatalf("seed redis key error: %v", err)
	}
	if err := rdb.Set(context.Background(), "user:8:current_conversation", "conv-8", 0).Err(); err != nil {
		t.Fatalf("seed redis key error: %v", err)
	}
	if err := rdb.Set(context.Background(), "conversation:conv-8", "[]", 0).Err(); err != nil {
		t.Fatalf("seed unrelated key error: %v", err)
	}

	mappings, err := repo.GetAllUserConversationMappings(context.Background())
	if err != nil {
		t.Fatalf("GetAllUserConversationMappings() error = %v", err)
	}
	if len(mappings) != 2 {
		t.Fatalf("expected 2 mappings, got %+v", mappings)
	}
	if mappings[3] != "conv-3" || mappings[8] != "conv-8" {
		t.Fatalf("unexpected mappings: %+v", mappings)
	}
}

func TestParseUserIDFromConversationKey(t *testing.T) {
	userID, err := parseUserIDFromConversationKey("user:9:current_conversation")
	if err != nil {
		t.Fatalf("parseUserIDFromConversationKey() error = %v", err)
	}
	if userID != 9 {
		t.Fatalf("expected userID=9, got %d", userID)
	}
}
