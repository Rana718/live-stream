package aiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Claude is a minimal Anthropic Messages API client.
// Uses the REST API directly to avoid pulling the SDK as a required dep.
type Claude struct {
	apiKey    string
	model     string
	maxTokens int
	endpoint  string
	http      *http.Client
}

func NewClaude(apiKey, model string, maxTokens int) *Claude {
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	if maxTokens <= 0 {
		maxTokens = 2048
	}
	return &Claude{
		apiKey:    apiKey,
		model:     model,
		maxTokens: maxTokens,
		endpoint:  "https://api.anthropic.com/v1/messages",
		http:      &http.Client{Timeout: 60 * time.Second},
	}
}

type messageContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type message struct {
	Role    string           `json:"role"`
	Content []messageContent `json:"content"`
}

type request struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []message `json:"messages"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type response struct {
	ID         string         `json:"id"`
	Model      string         `json:"model"`
	Content    []contentBlock `json:"content"`
	StopReason string         `json:"stop_reason"`
	Error      *apiError      `json:"error,omitempty"`
}

type apiError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Ask sends a prompt with a system preamble and returns the assistant's text.
func (c *Claude) Ask(ctx context.Context, system, userPrompt string) (string, error) {
	if c.apiKey == "" {
		return "", errors.New("claude api key not configured")
	}

	body := request{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		System:    system,
		Messages: []message{
			{
				Role:    "user",
				Content: []messageContent{{Type: "text", Text: userPrompt}},
			},
		},
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var parsed response
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("decode claude response: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("claude api error: %s: %s", parsed.Error.Type, parsed.Error.Message)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("claude api http %d: %s", resp.StatusCode, string(raw))
	}

	var sb bytes.Buffer
	for _, blk := range parsed.Content {
		if blk.Type == "text" {
			sb.WriteString(blk.Text)
		}
	}
	return sb.String(), nil
}

// Model returns the configured model id (useful for logging/storage).
func (c *Claude) Model() string { return c.model }
