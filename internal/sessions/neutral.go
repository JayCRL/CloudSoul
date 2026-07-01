// Package sessions 定义中立会话格式与各工具的 adapter。
package sessions

import "time"

// Role 是中立角色。
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Block 是一条消息内的内容块（文本 / 工具调用 / 工具结果）。
type Block struct {
	Type  string         `json:"type"`            // text | tool_use | tool_result
	Text  string         `json:"text,omitempty"`  // text / tool_result 文本
	Name  string         `json:"name,omitempty"`  // tool_use: 工具名
	Extra map[string]any `json:"extra,omitempty"` // 其余结构化字段
}

// Message 是一条规范化消息。
type Message struct {
	Role    Role      `json:"role"`
	Content []Block   `json:"content"`
	TS      time.Time `json:"ts,omitempty"`
}

// NeutralSession 是跨工具统一的会话表示。
type NeutralSession struct {
	SourceTool      string    `json:"source_tool"` // claude-code | codex
	SourceSessionID string    `json:"source_session_id"`
	CWD             string    `json:"cwd,omitempty"`
	Model           string    `json:"model,omitempty"`
	StartedAt       time.Time `json:"started_at,omitempty"`
	EndedAt         time.Time `json:"ended_at,omitempty"`
	Messages        []Message `json:"messages"`
}

// Adapter 把某工具的原始 JSONL 解析成中立会话。
type Adapter interface {
	Tool() string
	Parse(raw []byte) (*NeutralSession, error)
}

// parseTime 容忍 RFC3339 及带毫秒的两种常见时间串。
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// ParseTime 公开 parseTime（search.go 等包外调用）。
func ParseTime(s string) time.Time { return parseTime(s) }
