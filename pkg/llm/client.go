package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"pai_smart_go_v2/internal/config"
)

const (
	defaultLLMTimeout  = 120 * time.Second
	defaultTemperature = 0.2
	defaultTopP        = 0.9
	defaultMaxTokens   = 1024
	defaultAPIStyle    = "openai_compatible"
	TextMessageType    = 1
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type MessageWriter interface {
	WriteMessage(messageType int, data []byte) error
}

type Client interface {
	StreamChat(ctx context.Context, messages []Message, writer MessageWriter) error
}

type client struct {
	baseURL     string
	apiKey      string
	model       string
	temperature float64
	topP        float64
	maxTokens   int
	httpClient  *http.Client
}

type streamChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	Temperature float64   `json:"temperature"`
	TopP        float64   `json:"top_p"`
	MaxTokens   int       `json:"max_tokens"`
}

type streamChatChunk struct {
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

func NewClient(cfg config.LLMConfig) (Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("llm base_url is empty")
	}
	apiStyle := strings.TrimSpace(cfg.APIStyle)
	if apiStyle == "" {
		apiStyle = defaultAPIStyle
	}
	if apiStyle != defaultAPIStyle {
		return nil, fmt.Errorf("unsupported llm api_style: %s", apiStyle)
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("llm api_key is empty")
	}
	if !isASCII(cfg.APIKey) {
		return nil, fmt.Errorf("llm api_key contains non-ascii characters")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("llm model is empty")
	}

	timeout := defaultLLMTimeout
	if cfg.TimeoutSeconds > 0 {
		timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	}

	temperature := cfg.Generation.Temperature
	if temperature == 0 {
		temperature = defaultTemperature
	}
	topP := cfg.Generation.TopP
	if topP == 0 {
		topP = defaultTopP
	}
	maxTokens := cfg.Generation.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	return &client{
		baseURL:     baseURL,
		apiKey:      cfg.APIKey,
		model:       cfg.Model,
		temperature: temperature,
		topP:        topP,
		maxTokens:   maxTokens,
		httpClient:  &http.Client{Timeout: timeout},
	}, nil
}

func (c *client) StreamChat(ctx context.Context, messages []Message, writer MessageWriter) error {
	if len(messages) == 0 {
		return fmt.Errorf("llm messages are empty")
	}
	if writer == nil {
		return fmt.Errorf("llm writer is nil")
	}

	payload, err := json.Marshal(streamChatRequest{
		Model:       c.model,
		Messages:    messages,
		Stream:      true,
		Temperature: c.temperature,
		TopP:        c.topP,
		MaxTokens:   c.maxTokens,
	})
	if err != nil {
		return fmt.Errorf("marshal llm request failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create llm request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call llm api failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("llm api status=%d and read response failed: %w", resp.StatusCode, readErr)
		}
		return fmt.Errorf("llm api status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	reader := bufio.NewReader(resp.Body)
	for {
		line, readErr := reader.ReadString('\n')
		if errors.Is(readErr, io.EOF) && strings.TrimSpace(line) == "" {
			return nil
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return fmt.Errorf("read llm stream failed: %w", readErr)
		}

		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "data:") {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			continue
		}
		if payload == "[DONE]" {
			return nil
		}

		var chunk streamChatChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return fmt.Errorf("unmarshal llm stream chunk failed: %w", err)
		}
		if chunk.Error != nil && strings.TrimSpace(chunk.Error.Message) != "" {
			return fmt.Errorf("llm stream error: %s", chunk.Error.Message)
		}
		if len(chunk.Choices) == 0 {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			continue
		}

		content := chunk.Choices[0].Delta.Content
		if content != "" {
			if err := writer.WriteMessage(TextMessageType, []byte(content)); err != nil {
				return err
			}
		}

		if errors.Is(readErr, io.EOF) {
			return nil
		}
	}
}

func isASCII(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}
