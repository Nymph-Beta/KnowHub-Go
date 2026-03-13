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

func responseError(res *esapi.Response) string {
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Sprintf("status=%d read_body_error=%v", res.StatusCode, err)
	}
	return fmt.Sprintf("status=%d body=%s", res.StatusCode, strings.TrimSpace(string(body)))
}
