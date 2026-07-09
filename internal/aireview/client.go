package aireview

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://opencode.ai/zen/go/v1"
	defaultModel   = "deepseek-v4-flash"
	maxDiffBytes   = 100_000
)

type Config struct {
	Enabled  bool
	APIKey   string
	BaseURL  string
	Model    string
	Auto     bool
}

type Client struct {
	cfg    Config
	http   *http.Client
}

func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &Client{
		cfg: cfg,
		http: &http.Client{Timeout: 120 * time.Second},
	}
}

func (c *Client) Enabled() bool {
	return c.cfg.Enabled && c.cfg.APIKey != ""
}

func (c *Client) DefaultModel() string {
	return c.cfg.Model
}

func (c *Client) Auto() bool {
	return c.cfg.Auto
}

type ReviewInput struct {
	Title string
	Body  string
	Diff  string
	Model string
}

func (c *Client) Review(ctx context.Context, in ReviewInput) (string, error) {
	if !c.Enabled() {
		return "", fmt.Errorf("ai review is not configured")
	}
	model := in.Model
	if model == "" {
		model = c.cfg.Model
	}
	diff := in.Diff
	truncated := false
	if len(diff) > maxDiffBytes {
		diff = diff[:maxDiffBytes]
		truncated = true
	}
	userPrompt := fmt.Sprintf("Review this pull request.\n\nTitle: %s\n\nDescription:\n%s\n\nDiff:\n```\n%s\n```",
		in.Title, in.Body, diff)
	if truncated {
		userPrompt += "\n\n(Note: diff was truncated due to size limits.)"
	}

	reqBody := chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ai request failed (%d): %s", resp.StatusCode, truncate(string(body), 500))
	}
	var out chatResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("parse ai response: %w", err)
	}
	if len(out.Choices) == 0 || out.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("empty ai response")
	}
	return out.Choices[0].Message.Content, nil
}

const systemPrompt = `You are an expert code reviewer. Analyze the pull request diff and provide actionable feedback.
Structure your review with:
1. Summary (1-2 sentences)
2. Issues found (bugs, security, performance) — cite file/line when possible
3. Suggestions for improvement
4. Verdict: APPROVE, REQUEST_CHANGES, or COMMENT

Be concise and focus on substantive issues. Skip style nitpicks unless they affect readability.`

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
