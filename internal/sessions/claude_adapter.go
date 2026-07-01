package sessions

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
)

// ClaudeAdapter 解析 Claude Code JSONL：~/.claude/projects/<盘符路径编码>/<session-uuid>.jsonl。
// 记录形如 {type:user|assistant|summary, message:{role,content,model}, sessionId, cwd, timestamp}；
// content 可能是字符串或 block 数组（text/thinking/tool_use/tool_result）。
type ClaudeAdapter struct{}

func (ClaudeAdapter) Tool() string { return "claude-code" }

type claudeRecord struct {
	Type      string          `json:"type"`
	Message   json.RawMessage `json:"message"`
	SessionID string          `json:"sessionId"`
	CWD       string          `json:"cwd"`
	Timestamp string          `json:"timestamp"`
}

type claudeMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
	Model   string          `json:"model"`
}

type claudeBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Thinking string          `json:"thinking"`
	Name     string          `json:"name"`
	Content  json.RawMessage `json:"content"` // tool_result 内容
}

func (a ClaudeAdapter) Parse(raw []byte) (*NeutralSession, error) {
	ns := &NeutralSession{SourceTool: a.Tool()}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 1024*1024), 32*1024*1024)

	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var rec claudeRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		if rec.Type != "user" && rec.Type != "assistant" {
			continue // summary 等元数据行跳过
		}
		var msg claudeMessage
		if json.Unmarshal(rec.Message, &msg) != nil {
			continue
		}
		if ns.SourceSessionID == "" {
			ns.SourceSessionID = rec.SessionID
		}
		if ns.CWD == "" {
			ns.CWD = rec.CWD
		}
		if msg.Model != "" {
			ns.Model = msg.Model
		}
		ts := parseTime(rec.Timestamp)
		if ns.StartedAt.IsZero() {
			ns.StartedAt = ts
		}

		blocks := parseClaudeContent(msg.Content)
		if len(blocks) == 0 {
			continue
		}
		role := RoleUser
		if msg.Role == "assistant" {
			role = RoleAssistant
		}
		ns.Messages = append(ns.Messages, Message{Role: role, Content: blocks, TS: ts})
	}
	if n := len(ns.Messages); n > 0 {
		ns.EndedAt = ns.Messages[n-1].TS
	}
	return ns, sc.Err()
}

// parseClaudeContent 处理 content 为字符串或 block 数组两种形态。
func parseClaudeContent(raw json.RawMessage) []Block {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if strings.TrimSpace(s) == "" {
			return nil
		}
		return []Block{{Type: "text", Text: s}}
	}
	var arr []claudeBlock
	if json.Unmarshal(raw, &arr) != nil {
		return nil
	}
	var out []Block
	for _, b := range arr {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" {
				out = append(out, Block{Type: "text", Text: b.Text})
			}
		case "thinking":
			if strings.TrimSpace(b.Thinking) != "" {
				out = append(out, Block{Type: "thinking", Text: b.Thinking})
			}
		case "tool_use":
			out = append(out, Block{Type: "tool_use", Name: b.Name})
		case "tool_result":
			if txt := flattenText(b.Content); strings.TrimSpace(txt) != "" {
				out = append(out, Block{Type: "tool_result", Text: txt})
			}
		}
	}
	return out
}

// flattenText 把 tool_result 的 content（字符串或 [{type:text,text}]）压成纯文本。
func flattenText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var arr []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &arr) == nil {
		var sb strings.Builder
		for _, e := range arr {
			sb.WriteString(e.Text)
		}
		return sb.String()
	}
	return ""
}
