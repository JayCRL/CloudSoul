// Package ai 是一个最小的 OpenAI 兼容 chat 客户端，用于 handoff 压缩与习惯提炼。
// baseURL 约定为不含 /chat/completions 的根（如 https://host/v1），全部走 env 配置。
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	baseURL string
	token   string
	model   string
	hc      *http.Client
}

// New 返回一个 client；baseURL 为空时返回 nil，调用方据此降级。
func New(baseURL, token, model string) *Client {
	if baseURL == "" {
		return nil
	}
	return &Client{baseURL: baseURL, token: token, model: model, hc: &http.Client{Timeout: 90 * time.Second}}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type chatReq struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens,omitempty"`
}
type chatResp struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// Complete 发一次 chat completion，返回助手文本。
func (c *Client) Complete(ctx context.Context, system, user string) (string, error) {
	reqBody, _ := json.Marshal(chatReq{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		MaxTokens: 2000,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("ai http %d: %s", resp.StatusCode, string(b))
	}
	var cr chatResp
	if err := json.Unmarshal(b, &cr); err != nil {
		return "", err
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("ai: empty choices")
	}
	return cr.Choices[0].Message.Content, nil
}
