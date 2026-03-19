package llm

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"pai_smart_go_v2/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type fakeWriter struct {
	chunks []string
}

func (w *fakeWriter) WriteMessage(_ int, data []byte) error {
	w.chunks = append(w.chunks, string(data))
	return nil
}

func TestClientStreamChatSuccess(t *testing.T) {
	c := &client{
		baseURL:     "http://llm.local",
		apiKey:      "secret",
		model:       "deepseek-chat",
		temperature: 0.2,
		topP:        0.9,
		maxTokens:   512,
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/chat/completions" {
					t.Fatalf("unexpected path: %s", req.URL.Path)
				}
				if got := req.Header.Get("Authorization"); got != "Bearer secret" {
					t.Fatalf("unexpected authorization header: %s", got)
				}
				body := strings.Join([]string{
					"data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}",
					"",
					"data: {\"choices\":[{\"delta\":{\"content\":\"世界\"}}]}",
					"",
					"data: [DONE]",
					"",
				}, "\n")
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	}

	writer := &fakeWriter{}
	if err := c.StreamChat(context.Background(), []Message{{Role: "user", Content: "你好"}}, writer); err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}

	if got := strings.Join(writer.chunks, ""); got != "你好世界" {
		t.Fatalf("unexpected chunks: %q", got)
	}
}

func TestClientStreamChatStatusError(t *testing.T) {
	c := &client{
		baseURL: "http://llm.local",
		apiKey:  "secret",
		model:   "deepseek-chat",
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(`{"error":"bad request"}`)),
				Header:     make(http.Header),
			}, nil
		})},
	}

	err := c.StreamChat(context.Background(), []Message{{Role: "user", Content: "你好"}}, &fakeWriter{})
	if err == nil || !strings.Contains(err.Error(), "status=400") {
		t.Fatalf("expected status error, got %v", err)
	}
}

func TestNewClientUnsupportedAPIStyle(t *testing.T) {
	_, err := NewClient(config.LLMConfig{
		APIStyle: "anthropic_messages",
		APIKey:   "secret",
		BaseURL:  "http://llm.local",
		Model:    "demo",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported llm api_style") {
		t.Fatalf("expected unsupported api_style error, got %v", err)
	}
}

func TestNewClientRejectsNonASCIIAPIKey(t *testing.T) {
	_, err := NewClient(config.LLMConfig{
		APIStyle: "openai_compatible",
		APIKey:   "sk-demoß",
		BaseURL:  "http://llm.local",
		Model:    "demo",
	})
	if err == nil || !strings.Contains(err.Error(), "non-ascii") {
		t.Fatalf("expected non-ascii api_key error, got %v", err)
	}
}
