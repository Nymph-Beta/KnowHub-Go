package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/pkg/token"

	"github.com/go-redis/redis/v8"
)

const (
	defaultConversationTTL   = 7 * 24 * time.Hour
	defaultConversationLimit = 20
)

type ConversationRepository interface {
	GetConversationID(ctx context.Context, userID uint) (string, error)
	GetOrCreateConversationID(ctx context.Context, userID uint) (string, error)
	GetConversationHistory(ctx context.Context, conversationID string) ([]model.ChatMessage, error)
	UpdateConversationHistory(ctx context.Context, conversationID string, messages []model.ChatMessage) error
	GetAllUserConversationMappings(ctx context.Context) (map[uint]string, error)
}

type conversationRepository struct {
	rdb          *redis.Client
	ttl          time.Duration
	historyLimit int
}

func NewConversationRepository(rdb *redis.Client) ConversationRepository {
	return &conversationRepository{
		rdb:          rdb,
		ttl:          defaultConversationTTL,
		historyLimit: defaultConversationLimit,
	}
}

func (r *conversationRepository) GetOrCreateConversationID(ctx context.Context, userID uint) (string, error) {
	if r.rdb == nil || userID == 0 {
		return "", fmt.Errorf("conversation repository is not ready")
	}

	conversationID, err := r.GetConversationID(ctx, userID)
	switch {
	case err != nil:
		return "", err
	case strings.TrimSpace(conversationID) != "":
		if expireErr := r.rdb.Expire(ctx, currentConversationKey(userID), r.ttl).Err(); expireErr != nil {
			return "", fmt.Errorf("refresh current conversation ttl failed: %w", expireErr)
		}
		return conversationID, nil
	}

	conversationID = token.GenerateRandomString(16)
	if err := r.rdb.Set(ctx, currentConversationKey(userID), conversationID, r.ttl).Err(); err != nil {
		return "", fmt.Errorf("set current conversation failed: %w", err)
	}
	return conversationID, nil
}

func (r *conversationRepository) GetConversationID(ctx context.Context, userID uint) (string, error) {
	if r.rdb == nil || userID == 0 {
		return "", fmt.Errorf("conversation repository is not ready")
	}

	conversationID, err := r.rdb.Get(ctx, currentConversationKey(userID)).Result()
	switch {
	case err == redis.Nil:
		return "", nil
	case err != nil:
		return "", fmt.Errorf("get current conversation failed: %w", err)
	default:
		return strings.TrimSpace(conversationID), nil
	}
}

func (r *conversationRepository) GetConversationHistory(ctx context.Context, conversationID string) ([]model.ChatMessage, error) {
	if r.rdb == nil || strings.TrimSpace(conversationID) == "" {
		return []model.ChatMessage{}, nil
	}

	payload, err := r.rdb.Get(ctx, conversationHistoryKey(conversationID)).Result()
	switch {
	case err == redis.Nil:
		return []model.ChatMessage{}, nil
	case err != nil:
		return nil, fmt.Errorf("get conversation history failed: %w", err)
	}

	var history []model.ChatMessage
	if err := json.Unmarshal([]byte(payload), &history); err != nil {
		return nil, fmt.Errorf("unmarshal conversation history failed: %w", err)
	}
	return history, nil
}

func (r *conversationRepository) UpdateConversationHistory(ctx context.Context, conversationID string, messages []model.ChatMessage) error {
	if r.rdb == nil || strings.TrimSpace(conversationID) == "" {
		return fmt.Errorf("conversation repository is not ready")
	}

	trimmed := trimConversationHistory(messages, r.historyLimit)
	payload, err := json.Marshal(trimmed)
	if err != nil {
		return fmt.Errorf("marshal conversation history failed: %w", err)
	}

	if err := r.rdb.Set(ctx, conversationHistoryKey(conversationID), payload, r.ttl).Err(); err != nil {
		return fmt.Errorf("save conversation history failed: %w", err)
	}
	return nil
}

func (r *conversationRepository) GetAllUserConversationMappings(ctx context.Context) (map[uint]string, error) {
	if r.rdb == nil {
		return nil, fmt.Errorf("conversation repository is not ready")
	}

	result := make(map[uint]string)
	var cursor uint64

	for {
		keys, nextCursor, err := r.rdb.Scan(ctx, cursor, "user:*:current_conversation", 100).Result()
		if err != nil {
			return nil, fmt.Errorf("scan current conversation keys failed: %w", err)
		}

		if len(keys) > 0 {
			values, err := r.rdb.MGet(ctx, keys...).Result()
			if err != nil {
				return nil, fmt.Errorf("get scanned conversation values failed: %w", err)
			}

			for i, key := range keys {
				userID, parseErr := parseUserIDFromConversationKey(key)
				if parseErr != nil {
					continue
				}
				value, ok := values[i].(string)
				if !ok || strings.TrimSpace(value) == "" {
					continue
				}
				result[userID] = strings.TrimSpace(value)
			}
		}

		if nextCursor == 0 {
			break
		}
		cursor = nextCursor
	}

	return result, nil
}

func currentConversationKey(userID uint) string {
	return fmt.Sprintf("user:%d:current_conversation", userID)
}

func conversationHistoryKey(conversationID string) string {
	return fmt.Sprintf("conversation:%s", conversationID)
}

func trimConversationHistory(messages []model.ChatMessage, limit int) []model.ChatMessage {
	if len(messages) == 0 {
		return []model.ChatMessage{}
	}
	if limit <= 0 || len(messages) <= limit {
		return messages
	}
	return messages[len(messages)-limit:]
}

func parseUserIDFromConversationKey(key string) (uint, error) {
	trimmed := strings.TrimSpace(key)
	if !strings.HasPrefix(trimmed, "user:") || !strings.HasSuffix(trimmed, ":current_conversation") {
		return 0, fmt.Errorf("invalid conversation key: %s", key)
	}

	userIDPart := strings.TrimPrefix(trimmed, "user:")
	userIDPart = strings.TrimSuffix(userIDPart, ":current_conversation")
	parsed, err := strconv.ParseUint(userIDPart, 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(parsed), nil
}
