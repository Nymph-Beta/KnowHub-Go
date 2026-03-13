package embedding

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestClient_CreateEmbedding(t *testing.T) {
	client := &client{
		baseURL:    "http://embedding.local",
		apiKey:     "test-key",
		model:      "text-embedding-v4",
		dimensions: 2,
		httpClient: &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/embeddings" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
				t.Fatalf("unexpected authorization header: %s", got)
			}

			var req map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request failed: %v", err)
			}
			if req["model"] != "text-embedding-v4" {
				t.Fatalf("unexpected model: %v", req["model"])
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"data":[{"embedding":[0.1,0.2]}]}`)),
			}, nil
		})},
	}

	vector, err := client.CreateEmbedding(context.Background(), "hello")
	if err != nil {
		t.Fatalf("CreateEmbedding() error = %v", err)
	}
	if len(vector) != 2 || vector[0] != 0.1 || vector[1] != 0.2 {
		t.Fatalf("unexpected embedding vector: %+v", vector)
	}
}

func TestClient_CreateEmbedding_DimensionMismatch(t *testing.T) {
	client := &client{
		baseURL:    "http://embedding.local",
		apiKey:     "test-key",
		model:      "text-embedding-v4",
		dimensions: 2,
		httpClient: &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"data":[{"embedding":[0.1]}]}`)),
			}, nil
		})},
	}

	if _, err := client.CreateEmbedding(context.Background(), "hello"); err == nil {
		t.Fatalf("expected dimension mismatch error")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
