package ai

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/JayCRL/CloudSoul/internal/sessions"
)

const habitSuggestPrompt = `你是编程习惯分析师。分析以下 AI 编程会话的对话记录，提炼可作为「习惯层」的规则或偏好。每条控制在1-3句话，用简体中文。

输出的每条一行JSON（不要数组包裹）：
{"scope_type":"user|tool|model|workspace|ws_tool|ws_model","scope_key":"codex|opus|项目名等","content":"这里写习惯内容"}

scope_type含义：
- user: 用户级通用偏好（如语言、风格、决策习惯）
- tool: 某AI工具通用偏好（claude-code/codex）
- model: 某模型的偏好（opus/gpt-5.5等）
- workspace: 项目通用
- ws_tool/ws_model: 交叉特化

只提炼会持续有效的规律，不重复显而易见的。无可固化习惯则输出空。content要可直接作为习惯层指令。`

// Suggestion 是一条解析后的 AI 习惯建议。
type Suggestion struct {
	ScopeType string `json:"scope_type"`
	ScopeKey  string `json:"scope_key"`
	Content   string `json:"content"`
}

// SuggestHabits 从采样消息中 AI 提炼习惯候选。client 为 nil 时返回空。
func SuggestHabits(ctx context.Context, client *Client, msgs []sessions.Message) []Suggestion {
	if client == nil || len(msgs) == 0 {
		return nil
	}
	text := renderSuggestInput(msgs)
	resp, err := client.Complete(ctx, habitSuggestPrompt, text)
	if err != nil || strings.TrimSpace(resp) == "" {
		return nil
	}
	var out []Suggestion
	for _, line := range strings.Split(resp, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "[]" {
			continue
		}
		var s Suggestion
		if json.Unmarshal([]byte(line), &s) != nil {
			continue
		}
		if s.ScopeType == "" || s.Content == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

func renderSuggestInput(msgs []sessions.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		b.WriteString(string(m.Role))
		b.WriteString(": ")
		for _, blk := range m.Content {
			if blk.Type == "text" || blk.Type == "tool_result" {
				b.WriteString(blk.Text)
				b.WriteString(" ")
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}
