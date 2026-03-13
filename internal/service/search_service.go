package service

import (
	"context"
	"strings"
	"unicode"

	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/pkg/embedding"
	"pai_smart_go_v2/pkg/es"
	"pai_smart_go_v2/pkg/log"
)

const (
	defaultSearchTopK       = 10
	maxSearchTopK           = 50
	knnRecallMultiplier     = 30
	numCandidatesMultiplier = 60
	rescoreWindowMultiplier = 5
)

var searchStopwords = []string{
	"请问", "请教", "帮我", "帮忙", "一下", "一下子", "一下吧",
	"什么是", "是什么", "如何", "怎么", "怎样", "为什么",
	"有关", "关于", "介绍", "一下", "吗", "呢", "啊", "呀", "吧",
}

type SearchService interface {
	HybridSearch(ctx context.Context, query string, topK int, user *model.User) ([]model.SearchResponseDTO, error)
}

type searchUserOrgTagProvider interface {
	GetUserEffectiveOrgTags(userID uint) ([]model.OrganizationTag, error)
}

type searchUploadRepository interface {
	FindBatchByMD5s(fileMD5s []string) ([]model.FileUpload, error)
}

type searchService struct {
	embeddingClient embedding.Client
	esClient        es.Client
	userService     searchUserOrgTagProvider
	uploadRepo      searchUploadRepository
}

func NewSearchService(
	embeddingClient embedding.Client,
	esClient es.Client,
	userService searchUserOrgTagProvider,
	uploadRepo searchUploadRepository,
) SearchService {
	return &searchService{
		embeddingClient: embeddingClient,
		esClient:        esClient,
		userService:     userService,
		uploadRepo:      uploadRepo,
	}
}

func (s *searchService) HybridSearch(ctx context.Context, query string, topK int, user *model.User) ([]model.SearchResponseDTO, error) {
	if s.embeddingClient == nil || s.esClient == nil || s.userService == nil || s.uploadRepo == nil {
		return nil, ErrInternal
	}
	if user == nil {
		return nil, ErrInvalidInput
	}

	rawQuery := strings.TrimSpace(query)
	if rawQuery == "" {
		return nil, ErrInvalidInput
	}

	topK = normalizeTopK(topK)
	normalizedQuery, phraseQuery := normalizeQuery(rawQuery)
	if normalizedQuery == "" {
		normalizedQuery = sanitizeQuery(rawQuery)
		phraseQuery = normalizedQuery
	}
	if normalizedQuery == "" {
		return nil, ErrInvalidInput
	}

	orgTags, err := s.userService.GetUserEffectiveOrgTags(user.ID)
	if err != nil {
		return nil, err
	}

	queryVector, err := s.embeddingClient.CreateEmbedding(ctx, rawQuery)
	if err != nil {
		log.Errorf("HybridSearch: create query embedding failed: %v", err)
		return nil, ErrInternal
	}

	hits, err := s.esClient.SearchDocuments(ctx, es.SearchRequest{
		QueryVector:        queryVector,
		Query:              normalizedQuery,
		Phrase:             phraseQuery,
		TopK:               topK,
		KNNK:               topK * knnRecallMultiplier,
		NumCandidates:      topK * numCandidatesMultiplier,
		RescoreWindow:      topK * rescoreWindowMultiplier,
		QueryWeight:        0.35,
		RescoreQueryWeight: 1.25,
		UserID:             user.ID,
		OrgTags:            extractOrgTagIDs(orgTags),
	})
	if err != nil {
		log.Errorf("HybridSearch: elasticsearch query failed: %v", err)
		return nil, ErrInternal
	}
	if len(hits) == 0 {
		return []model.SearchResponseDTO{}, nil
	}

	uploads, err := s.uploadRepo.FindBatchByMD5s(uniqueFileMD5s(hits))
	if err != nil {
		log.Errorf("HybridSearch: query uploads by md5 failed: %v", err)
		return nil, ErrInternal
	}

	fileNameByMD5 := make(map[string]string, len(uploads))
	for _, upload := range uploads {
		if _, exists := fileNameByMD5[upload.FileMD5]; exists {
			continue
		}
		fileNameByMD5[upload.FileMD5] = upload.FileName
	}

	results := make([]model.SearchResponseDTO, 0, len(hits))
	for _, hit := range hits {
		results = append(results, model.SearchResponseDTO{
			FileMD5:     hit.Source.FileMD5,
			FileName:    fileNameByMD5[hit.Source.FileMD5],
			ChunkID:     hit.Source.ChunkID,
			TextContent: hit.Source.TextContent,
			Score:       hit.Score,
			UserID:      hit.Source.UserID,
			OrgTag:      hit.Source.OrgTag,
			IsPublic:    hit.Source.IsPublic,
		})
	}

	return results, nil
}

func normalizeQuery(query string) (normalized string, phrase string) {
	cleaned := strings.ToLower(strings.TrimSpace(query))
	for _, stopword := range searchStopwords {
		cleaned = strings.ReplaceAll(cleaned, stopword, " ")
	}
	cleaned = sanitizeQuery(cleaned)
	if cleaned == "" {
		return "", ""
	}
	return cleaned, cleaned
}

func sanitizeQuery(query string) string {
	var builder strings.Builder
	builder.Grow(len(query))

	lastSpace := false
	for _, r := range query {
		switch {
		case unicode.Is(unicode.Han, r), unicode.IsLetter(r), unicode.IsDigit(r):
			builder.WriteRune(r)
			lastSpace = false
		case !lastSpace:
			builder.WriteByte(' ')
			lastSpace = true
		}
	}

	return strings.Join(strings.Fields(builder.String()), " ")
}

func normalizeTopK(topK int) int {
	switch {
	case topK <= 0:
		return defaultSearchTopK
	case topK > maxSearchTopK:
		return maxSearchTopK
	default:
		return topK
	}
}

func extractOrgTagIDs(tags []model.OrganizationTag) []string {
	ids := make([]string, 0, len(tags))
	for _, tag := range tags {
		tagID := strings.TrimSpace(tag.TagID)
		if tagID == "" {
			continue
		}
		ids = append(ids, tagID)
	}
	return ids
}

func uniqueFileMD5s(hits []es.SearchHit) []string {
	md5s := make([]string, 0, len(hits))
	seen := make(map[string]struct{}, len(hits))
	for _, hit := range hits {
		fileMD5 := strings.TrimSpace(hit.Source.FileMD5)
		if fileMD5 == "" {
			continue
		}
		if _, exists := seen[fileMD5]; exists {
			continue
		}
		seen[fileMD5] = struct{}{}
		md5s = append(md5s, fileMD5)
	}
	return md5s
}
