package tika

import (
	"context"
	"io"
	"net/http"
	"pai_smart_go_v2/internal/config"
	"strings"
	"testing"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestNewClient_EmptyBaseURL(t *testing.T) {
	_, err := NewClient(config.TikaConfig{BaseURL: ""})
	if err == nil {
		t.Fatalf("expected error for empty base_url")
	}
}

func TestExtractText_Success(t *testing.T) {
	client, err := NewClient(config.TikaConfig{BaseURL: "http://tika.local", TimeoutSeconds: 5})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPut || r.URL.Path != "/tika" {
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("hello world")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	text, err := client.ExtractText(context.Background(), strings.NewReader("pdf-bytes"), "a.pdf")
	if err != nil {
		t.Fatalf("ExtractText() error = %v", err)
	}
	if text != "hello world" {
		t.Fatalf("unexpected text: %s", text)
	}
}

func TestExtractText_Non200(t *testing.T) {
	client, err := NewClient(config.TikaConfig{BaseURL: "http://tika.local", TimeoutSeconds: 5})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader("bad file")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	_, err = client.ExtractText(context.Background(), strings.NewReader("bad"), "a.pdf")
	if err == nil {
		t.Fatalf("expected non-200 error")
	}
}

func TestDetectContentType(t *testing.T) {
	if got := detectContentType("a.pdf"); got == "application/octet-stream" {
		t.Fatalf("expected specific content-type for pdf, got %s", got)
	}
	if got := detectContentType("a.unknown"); got != "application/octet-stream" {
		t.Fatalf("expected fallback content-type, got %s", got)
	}
}
