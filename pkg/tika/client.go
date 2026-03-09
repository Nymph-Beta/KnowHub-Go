package tika

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"pai_smart_go_v2/internal/config"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(cfg config.TikaConfig) (*Client, error) {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("tika base_url is empty")
	}

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if cfg.TimeoutSeconds <= 0 {
		timeout = 10 * time.Second
	}

	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

func (c *Client) ExtractText(ctx context.Context, reader io.Reader, fileName string) (string, error) {
	if reader == nil {
		return "", fmt.Errorf("reader is nil")
	}

	contentType := detectContentType(fileName)
	endpoint := c.baseURL + "/tika"

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, reader)
	if err != nil {
		return "", fmt.Errorf("create tika request failed: %w", err)
	}
	req.Header.Set("Accept", "text/plain")
	req.Header.Set("Content-Type", contentType)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call tika failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read tika response failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tika response status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return string(body), nil
}

func detectContentType(fileName string) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	if ext == "" {
		return "application/octet-stream"
	}
	contentType := mime.TypeByExtension(ext)
	if strings.TrimSpace(contentType) == "" {
		return "application/octet-stream"
	}
	return contentType
}
