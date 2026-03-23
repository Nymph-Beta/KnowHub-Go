package es

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"pai_smart_go_v2/internal/config"
	"pai_smart_go_v2/internal/model"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

const (
	defaultIndexName  = "knowledge_base"
	defaultVectorDims = 2048
	defaultAnalyzer   = "standard"
)

type Client interface {
	EnsureIndex(ctx context.Context) error
	BulkIndexDocuments(ctx context.Context, docs []model.EsDocument) error
	SearchDocuments(ctx context.Context, req SearchRequest) ([]SearchHit, error)
	DeleteDocumentsByFileMD5(ctx context.Context, fileMD5 string) error
	IndexName() string
}

type client struct {
	raw *elasticsearch.Client
	cfg config.ElasticsearchConfig
}

type bulkResponse struct {
	Errors bool `json:"errors"`
	Items  []map[string]struct {
		Status int             `json:"status"`
		Error  json.RawMessage `json:"error"`
	} `json:"items"`
}

type SearchRequest struct {
	QueryVector        []float32
	Query              string
	Phrase             string
	TopK               int
	KNNK               int
	NumCandidates      int
	RescoreWindow      int
	QueryWeight        float64
	RescoreQueryWeight float64
	UserID             uint
	OrgTags            []string
}

type SearchHit struct {
	Score  float64
	Source model.EsDocument
}

type searchResponse struct {
	Hits struct {
		Hits []struct {
			Score  float64          `json:"_score"`
			Source model.EsDocument `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

func NewClient(cfg config.ElasticsearchConfig) (Client, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}

	raw, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: normalized.Addresses,
		Username:  normalized.Username,
		Password:  normalized.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("create elasticsearch client failed: %w", err)
	}

	return &client{
		raw: raw,
		cfg: normalized,
	}, nil
}

func (c *client) IndexName() string {
	return c.cfg.IndexName
}

func (c *client) EnsureIndex(ctx context.Context) error {
	res, err := c.raw.Indices.Exists([]string{c.cfg.IndexName}, c.raw.Indices.Exists.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("check index exists failed: %w", err)
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case 200:
		return nil
	case 404:
		return c.createIndex(ctx)
	default:
		return fmt.Errorf("check index exists failed: %s", responseError(res))
	}
}

func (c *client) BulkIndexDocuments(ctx context.Context, docs []model.EsDocument) error {
	if len(docs) == 0 {
		return nil
	}

	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	for _, doc := range docs {
		meta := map[string]map[string]string{
			"index": {
				"_id": doc.VectorID,
			},
		}
		if err := encoder.Encode(meta); err != nil {
			return fmt.Errorf("encode bulk metadata failed: %w", err)
		}
		if err := encoder.Encode(doc); err != nil {
			return fmt.Errorf("encode bulk document failed: %w", err)
		}
	}

	opts := []func(*esapi.BulkRequest){
		c.raw.Bulk.WithContext(ctx),
		c.raw.Bulk.WithIndex(c.cfg.IndexName),
	}
	if c.cfg.RefreshOnWrite {
		opts = append(opts, c.raw.Bulk.WithRefresh("true"))
	}

	res, err := c.raw.Bulk(&body, opts...)
	if err != nil {
		return fmt.Errorf("bulk index documents failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("bulk index documents failed: %s", responseError(res))
	}

	var parsed bulkResponse
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return fmt.Errorf("decode bulk response failed: %w", err)
	}
	if !parsed.Errors {
		return nil
	}

	for _, item := range parsed.Items {
		for action, result := range item {
			if result.Status >= 300 {
				if len(result.Error) > 0 {
					return fmt.Errorf("bulk %s failed: status=%d error=%s", action, result.Status, strings.TrimSpace(string(result.Error)))
				}
				return fmt.Errorf("bulk %s failed: status=%d", action, result.Status)
			}
		}
	}

	return fmt.Errorf("bulk index documents failed with unknown item error")
}

func (c *client) SearchDocuments(ctx context.Context, req SearchRequest) ([]SearchHit, error) {
	if len(req.QueryVector) == 0 {
		return nil, fmt.Errorf("query vector is empty")
	}
	if req.TopK <= 0 {
		return nil, fmt.Errorf("topK must be greater than 0")
	}

	body, err := json.Marshal(buildSearchBody(req))
	if err != nil {
		return nil, fmt.Errorf("marshal search body failed: %w", err)
	}

	res, err := c.raw.Search(
		c.raw.Search.WithContext(ctx),
		c.raw.Search.WithIndex(c.cfg.IndexName),
		c.raw.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return nil, fmt.Errorf("search documents failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("search documents failed: %s", responseError(res))
	}

	var parsed searchResponse
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode search response failed: %w", err)
	}

	hits := make([]SearchHit, 0, len(parsed.Hits.Hits))
	for _, hit := range parsed.Hits.Hits {
		hits = append(hits, SearchHit{
			Score:  hit.Score,
			Source: hit.Source,
		})
	}
	return hits, nil
}

func (c *client) DeleteDocumentsByFileMD5(ctx context.Context, fileMD5 string) error {
	fileMD5 = strings.TrimSpace(fileMD5)
	if fileMD5 == "" {
		return fmt.Errorf("file_md5 is empty")
	}

	body, err := json.Marshal(map[string]interface{}{
		"query": map[string]interface{}{
			"term": map[string]interface{}{
				"file_md5": fileMD5,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("marshal delete-by-query body failed: %w", err)
	}

	res, err := c.raw.DeleteByQuery(
		[]string{c.cfg.IndexName},
		bytes.NewReader(body),
		c.raw.DeleteByQuery.WithContext(ctx),
		c.raw.DeleteByQuery.WithRefresh(true),
	)
	if err != nil {
		return fmt.Errorf("delete documents by file_md5 failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("delete documents by file_md5 failed: %s", responseError(res))
	}
	return nil
}

func (c *client) createIndex(ctx context.Context) error {
	body, err := json.Marshal(buildIndexMapping(c.cfg))
	if err != nil {
		return fmt.Errorf("marshal index mapping failed: %w", err)
	}

	res, err := c.raw.Indices.Create(
		c.cfg.IndexName,
		c.raw.Indices.Create.WithContext(ctx),
		c.raw.Indices.Create.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return fmt.Errorf("create index failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("create index failed: %s", responseError(res))
	}
	return nil
}

func normalizeConfig(cfg config.ElasticsearchConfig) (config.ElasticsearchConfig, error) {
	if len(cfg.Addresses) == 0 {
		return cfg, fmt.Errorf("elasticsearch addresses are empty")
	}
	if strings.TrimSpace(cfg.IndexName) == "" {
		cfg.IndexName = defaultIndexName
	}
	if cfg.VectorDims <= 0 {
		cfg.VectorDims = defaultVectorDims
	}
	if strings.TrimSpace(cfg.Analyzer) == "" {
		cfg.Analyzer = defaultAnalyzer
	}
	if strings.TrimSpace(cfg.SearchAnalyzer) == "" {
		cfg.SearchAnalyzer = cfg.Analyzer
	}

	return cfg, nil
}

func buildIndexMapping(cfg config.ElasticsearchConfig) map[string]interface{} {
	return map[string]interface{}{
		"mappings": map[string]interface{}{
			"properties": map[string]interface{}{
				"vector_id": map[string]interface{}{
					"type": "keyword",
				},
				"file_md5": map[string]interface{}{
					"type": "keyword",
				},
				"chunk_id": map[string]interface{}{
					"type": "integer",
				},
				"text_content": map[string]interface{}{
					"type":            "text",
					"analyzer":        cfg.Analyzer,
					"search_analyzer": cfg.SearchAnalyzer,
				},
				"vector": map[string]interface{}{
					"type":       "dense_vector",
					"dims":       cfg.VectorDims,
					"index":      true,
					"similarity": "cosine",
				},
				"model_version": map[string]interface{}{
					"type": "keyword",
				},
				"user_id": map[string]interface{}{
					"type": "long",
				},
				"org_tag": map[string]interface{}{
					"type": "keyword",
				},
				"is_public": map[string]interface{}{
					"type": "boolean",
				},
			},
		},
	}
}

func buildSearchBody(req SearchRequest) map[string]interface{} {
	permissionFilter := buildPermissionFilter(req.UserID, req.OrgTags)
	textShould := buildTextShouldClauses(req.Query, req.Phrase)

	body := map[string]interface{}{
		"size": req.TopK,
		"_source": []string{
			"vector_id",
			"file_md5",
			"chunk_id",
			"text_content",
			"model_version",
			"user_id",
			"org_tag",
			"is_public",
		},
		"knn": map[string]interface{}{
			"field":          "vector",
			"query_vector":   req.QueryVector,
			"k":              positiveOrDefault(req.KNNK, req.TopK),
			"num_candidates": positiveOrDefault(req.NumCandidates, positiveOrDefault(req.KNNK, req.TopK)),
			"filter":         permissionFilter,
		},
		"query": buildQueryClause(permissionFilter, textShould),
	}

	if len(textShould) > 0 {
		body["rescore"] = map[string]interface{}{
			"window_size": positiveOrDefault(req.RescoreWindow, req.TopK),
			"query": map[string]interface{}{
				"query_weight":         positiveFloatOrDefault(req.QueryWeight, 0.35),
				"rescore_query_weight": positiveFloatOrDefault(req.RescoreQueryWeight, 1.25),
				"score_mode":           "total",
				"rescore_query": map[string]interface{}{
					"bool": map[string]interface{}{
						"should":               textShould,
						"minimum_should_match": 1,
					},
				},
			},
		}
	}

	return body
}

func buildPermissionFilter(userID uint, orgTags []string) map[string]interface{} {
	should := make([]interface{}, 0, 3)
	should = append(should,
		map[string]interface{}{"term": map[string]interface{}{"is_public": true}},
		map[string]interface{}{"term": map[string]interface{}{"user_id": userID}},
	)
	if len(orgTags) > 0 {
		should = append(should, map[string]interface{}{
			"terms": map[string]interface{}{
				"org_tag": orgTags,
			},
		})
	}

	return map[string]interface{}{
		"bool": map[string]interface{}{
			"should":               should,
			"minimum_should_match": 1,
		},
	}
}

func buildTextShouldClauses(query string, phrase string) []interface{} {
	query = strings.TrimSpace(query)
	phrase = strings.TrimSpace(phrase)
	if query == "" {
		return nil
	}

	should := []interface{}{
		map[string]interface{}{
			"match": map[string]interface{}{
				"text_content": map[string]interface{}{
					"query": query,
					"boost": 1.0,
				},
			},
		},
	}
	if phrase != "" {
		should = append(should, map[string]interface{}{
			"match_phrase": map[string]interface{}{
				"text_content": map[string]interface{}{
					"query": phrase,
					"boost": 2.0,
				},
			},
		})
	}
	return should
}

func buildQueryClause(permissionFilter map[string]interface{}, textShould []interface{}) map[string]interface{} {
	boolQuery := map[string]interface{}{
		"filter": []interface{}{permissionFilter},
	}
	if len(textShould) > 0 {
		boolQuery["must"] = []interface{}{
			map[string]interface{}{
				"bool": map[string]interface{}{
					"should":               textShould,
					"minimum_should_match": 1,
				},
			},
		}
	}

	return map[string]interface{}{
		"bool": boolQuery,
	}
}

func positiveOrDefault(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func positiveFloatOrDefault(value float64, fallback float64) float64 {
	if value > 0 {
		return value
	}
	return fallback
}

func responseError(res *esapi.Response) string {
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Sprintf("status=%d read_body_error=%v", res.StatusCode, err)
	}
	return fmt.Sprintf("status=%d body=%s", res.StatusCode, strings.TrimSpace(string(body)))
}
