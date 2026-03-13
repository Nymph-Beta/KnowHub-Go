package es

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"pai_smart_go_v2/internal/config"
	"pai_smart_go_v2/internal/model"

	"github.com/elastic/go-elasticsearch/v8"
)

func TestClient_EnsureIndex_CreatesMissingIndex(t *testing.T) {
	var createBody string
	raw, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{"http://es.local"},
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			switch {
			case r.Method == http.MethodHead && r.URL.Path == "/knowledge_base":
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Header:     http.Header{"X-Elastic-Product": []string{"Elasticsearch"}},
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			case r.Method == http.MethodPut && r.URL.Path == "/knowledge_base":
				body, readErr := io.ReadAll(r.Body)
				if readErr != nil {
					t.Fatalf("ReadAll() error = %v", readErr)
				}
				createBody = string(body)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"X-Elastic-Product": []string{"Elasticsearch"},
						"Content-Type":      []string{"application/json"},
					},
					Body: io.NopCloser(strings.NewReader(`{"acknowledged":true}`)),
				}, nil
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
				return nil, nil
			}
		}),
	})
	if err != nil {
		t.Fatalf("elasticsearch.NewClient() error = %v", err)
	}

	cfg, err := normalizeConfig(config.ElasticsearchConfig{
		Addresses:      []string{"http://es.local"},
		IndexName:      "knowledge_base",
		VectorDims:     2,
		Analyzer:       "standard",
		SearchAnalyzer: "standard",
	})
	if err != nil {
		t.Fatalf("normalizeConfig() error = %v", err)
	}

	client := &client{raw: raw, cfg: cfg}
	if err := client.EnsureIndex(context.Background()); err != nil {
		t.Fatalf("EnsureIndex() error = %v", err)
	}
	if !strings.Contains(createBody, `"dense_vector"`) || !strings.Contains(createBody, `"dims":2`) {
		t.Fatalf("unexpected index mapping body: %s", createBody)
	}
}

func TestClient_BulkIndexDocuments(t *testing.T) {
	var bulkBody string
	raw, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{"http://es.local"},
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost || r.URL.Path != "/knowledge_base/_bulk" {
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Fatalf("ReadAll() error = %v", readErr)
			}
			bulkBody = string(body)

			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"X-Elastic-Product": []string{"Elasticsearch"},
					"Content-Type":      []string{"application/json"},
				},
				Body: io.NopCloser(strings.NewReader(`{"errors":false,"items":[{"index":{"status":201}}]}`)),
			}, nil
		}),
	})
	if err != nil {
		t.Fatalf("elasticsearch.NewClient() error = %v", err)
	}

	cfg, err := normalizeConfig(config.ElasticsearchConfig{
		Addresses:      []string{"http://es.local"},
		IndexName:      "knowledge_base",
		VectorDims:     2,
		Analyzer:       "standard",
		SearchAnalyzer: "standard",
		RefreshOnWrite: true,
	})
	if err != nil {
		t.Fatalf("normalizeConfig() error = %v", err)
	}

	client := &client{raw: raw, cfg: cfg}
	err = client.BulkIndexDocuments(context.Background(), []model.EsDocument{
		{
			VectorID:     "md5_0",
			FileMD5:      "md5",
			ChunkID:      0,
			TextContent:  "hello",
			Vector:       []float32{0.1, 0.2},
			ModelVersion: "text-embedding-v4",
			UserID:       1,
			OrgTag:       "team-a",
			IsPublic:     true,
		},
	})
	if err != nil {
		t.Fatalf("BulkIndexDocuments() error = %v", err)
	}
	if !strings.Contains(bulkBody, `"index":{"_id":"md5_0"}`) {
		t.Fatalf("unexpected bulk body: %s", bulkBody)
	}
	if !strings.Contains(bulkBody, `"vector":[0.1,0.2]`) {
		t.Fatalf("unexpected bulk document body: %s", bulkBody)
	}
}

func TestClient_SearchDocuments(t *testing.T) {
	var searchBody string
	raw, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{"http://es.local"},
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost || r.URL.Path != "/knowledge_base/_search" {
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Fatalf("ReadAll() error = %v", readErr)
			}
			searchBody = string(body)

			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"X-Elastic-Product": []string{"Elasticsearch"},
					"Content-Type":      []string{"application/json"},
				},
				Body: io.NopCloser(strings.NewReader(`{
					"hits": {
						"hits": [
							{
								"_score": 7.5,
								"_source": {
									"vector_id": "md5_0",
									"file_md5": "md5",
									"chunk_id": 0,
									"text_content": "hello",
									"model_version": "text-embedding-v4",
									"user_id": 1,
									"org_tag": "team-a",
									"is_public": true
								}
							}
						]
					}
				}`)),
			}, nil
		}),
	})
	if err != nil {
		t.Fatalf("elasticsearch.NewClient() error = %v", err)
	}

	cfg, err := normalizeConfig(config.ElasticsearchConfig{
		Addresses:      []string{"http://es.local"},
		IndexName:      "knowledge_base",
		VectorDims:     2,
		Analyzer:       "standard",
		SearchAnalyzer: "standard",
	})
	if err != nil {
		t.Fatalf("normalizeConfig() error = %v", err)
	}

	client := &client{raw: raw, cfg: cfg}
	hits, err := client.SearchDocuments(context.Background(), SearchRequest{
		QueryVector:        []float32{0.1, 0.2},
		Query:              "go 并发",
		Phrase:             "go 并发",
		TopK:               5,
		KNNK:               150,
		NumCandidates:      300,
		RescoreWindow:      20,
		QueryWeight:        0.4,
		RescoreQueryWeight: 1.3,
		UserID:             1,
		OrgTags:            []string{"team-a", "team-b"},
	})
	if err != nil {
		t.Fatalf("SearchDocuments() error = %v", err)
	}
	if len(hits) != 1 || hits[0].Source.FileMD5 != "md5" || hits[0].Score != 7.5 {
		t.Fatalf("unexpected hits: %+v", hits)
	}
	if !strings.Contains(searchBody, `"knn"`) || !strings.Contains(searchBody, `"rescore"`) {
		t.Fatalf("unexpected search body: %s", searchBody)
	}
	if !strings.Contains(searchBody, `"org_tag":["team-a","team-b"]`) {
		t.Fatalf("unexpected permission filter in body: %s", searchBody)
	}
	if !strings.Contains(searchBody, `"match_phrase"`) {
		t.Fatalf("expected phrase query in body: %s", searchBody)
	}
}

func TestBuildSearchBody_QuerylessFallsBackToFilterOnly(t *testing.T) {
	body := buildSearchBody(SearchRequest{
		QueryVector: []float32{0.1, 0.2},
		TopK:        5,
		UserID:      9,
	})

	query := body["query"].(map[string]interface{})["bool"].(map[string]interface{})
	if _, ok := query["must"]; ok {
		t.Fatalf("unexpected must clause for empty text query: %+v", query)
	}
	if _, ok := body["rescore"]; ok {
		t.Fatalf("unexpected rescore for empty text query: %+v", body)
	}
}

func TestBuildIndexMapping_DefaultSearchAnalyzer(t *testing.T) {
	cfg, err := normalizeConfig(config.ElasticsearchConfig{
		Addresses:  []string{"http://localhost:9200"},
		IndexName:  "knowledge_base",
		VectorDims: 2,
		Analyzer:   "standard",
	})
	if err != nil {
		t.Fatalf("normalizeConfig() error = %v", err)
	}

	mapping := buildIndexMapping(cfg)
	properties := mapping["mappings"].(map[string]interface{})["properties"].(map[string]interface{})
	textContent := properties["text_content"].(map[string]interface{})

	if got := textContent["search_analyzer"]; got != "standard" {
		t.Fatalf("unexpected search analyzer: %v", got)
	}
	if got := properties["vector"].(map[string]interface{})["dims"]; got != 2 {
		t.Fatalf("unexpected vector dims: %v", got)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
