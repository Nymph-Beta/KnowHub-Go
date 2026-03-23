package service

import (
	"context"
	"sort"
	"time"

	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/internal/repository"
	"pai_smart_go_v2/pkg/log"
)

type ConversationAdminFilter struct {
	UserID    *uint
	StartTime *time.Time
	EndTime   *time.Time
}

type ConversationAdminRecord struct {
	ConversationID string    `json:"conversationId"`
	UserID         uint      `json:"userId"`
	Username       string    `json:"username"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"createdAt"`
}

type conversationUserFinder interface {
	FindByID(userID uint) (*model.User, error)
}

type ConversationService interface {
	GetConversationHistory(ctx context.Context, userID uint) ([]model.ChatMessage, error)
	GetAllConversations(ctx context.Context, filter ConversationAdminFilter) ([]ConversationAdminRecord, error)
}

type conversationService struct {
	repo       repository.ConversationRepository
	userFinder conversationUserFinder
}

func NewConversationService(repo repository.ConversationRepository, userFinder conversationUserFinder) ConversationService {
	return &conversationService{repo: repo, userFinder: userFinder}
}

func (s *conversationService) GetConversationHistory(ctx context.Context, userID uint) ([]model.ChatMessage, error) {
	if s.repo == nil {
		return nil, ErrServiceUnavailable
	}
	if userID == 0 {
		return nil, ErrInvalidInput
	}

	conversationID, err := s.repo.GetConversationID(ctx, userID)
	if err != nil {
		log.Errorf("GetConversationHistory: get conversation id failed: %v", err)
		return nil, ErrInternal
	}
	if conversationID == "" {
		return []model.ChatMessage{}, nil
	}

	history, err := s.repo.GetConversationHistory(ctx, conversationID)
	if err != nil {
		log.Errorf("GetConversationHistory: get history failed: %v", err)
		return nil, ErrInternal
	}
	return history, nil
}

func (s *conversationService) GetAllConversations(ctx context.Context, filter ConversationAdminFilter) ([]ConversationAdminRecord, error) {
	if s.repo == nil || s.userFinder == nil {
		return nil, ErrServiceUnavailable
	}
	if filter.StartTime != nil && filter.EndTime != nil && filter.StartTime.After(*filter.EndTime) {
		return nil, ErrInvalidInput
	}

	mappings := make(map[uint]string)
	if filter.UserID != nil {
		conversationID, err := s.repo.GetConversationID(ctx, *filter.UserID)
		if err != nil {
			log.Errorf("GetAllConversations: get conversation id by user failed: %v", err)
			return nil, ErrInternal
		}
		if conversationID == "" {
			return []ConversationAdminRecord{}, nil
		}
		mappings[*filter.UserID] = conversationID
	} else {
		allMappings, err := s.repo.GetAllUserConversationMappings(ctx)
		if err != nil {
			log.Errorf("GetAllConversations: get all mappings failed: %v", err)
			return nil, ErrInternal
		}
		mappings = allMappings
	}

	records := make([]ConversationAdminRecord, 0)
	for userID, conversationID := range mappings {
		user, err := s.userFinder.FindByID(userID)
		if err != nil || user == nil {
			continue
		}

		history, err := s.repo.GetConversationHistory(ctx, conversationID)
		if err != nil {
			continue
		}

		for _, message := range history {
			if filter.StartTime != nil && message.CreatedAt.Before(*filter.StartTime) {
				continue
			}
			if filter.EndTime != nil && message.CreatedAt.After(*filter.EndTime) {
				continue
			}
			records = append(records, ConversationAdminRecord{
				ConversationID: conversationID,
				UserID:         userID,
				Username:       user.Username,
				Role:           message.Role,
				Content:        message.Content,
				CreatedAt:      message.CreatedAt,
			})
		}
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt.Before(records[j].CreatedAt)
	})
	return records, nil
}
