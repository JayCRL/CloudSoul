package sessions

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
)

// CodexAdapter 解析 Codex rollout JSONL：~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl。
// 每行 {timestamp,type,payload}；真实对话在 type=="response_item" 的 message 里，
// 需过滤 developer/system 注入与 user 里的 <...> 系统包装块。多版本 schema 容错解析。
type CodexAdapter struct{}

func (CodexAdapter) Tool() string { return "codex" }

type codexRecord struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexMsgPayload struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"` // input_text | output_text
		Text string `json:"text"`
	} `json:"content"`
}

// systemTagPrefixes 标识 Codex 注入的系统/developer 包装块（过滤掉，只留真实对话）。
var systemTagPrefixes = []string{
	"<environment_context>", "<permissions", "<collaboration_mode>",
	"<apps_instructions>", "<skills_instructions>", "<plugins_instructions>",
	"<personality_spec>", "<user_instructions>",
}

func isCodexSystemInjection(text string) bool {
	t := strings.TrimSpace(text)
	for _, p := range systemTagPrefixes {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

func (a CodexAdapter) Parse(raw []byte) (*NeutralSession, error) {
	ns := &NeutralSession{SourceTool: a.Tool()}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 1024*1024), 32*1024*1024) // 容忍超长行

	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var rec codexRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue // 坏行跳过，尽量解析
		}
		switch rec.Type {
		case "session_meta":
			var m struct {
				ID        string `json:"id"`
				Timestamp string `json:"timestamp"`
				CWD       string `json:"cwd"`
			}
			if json.Unmarshal(rec.Payload, &m) == nil {
				ns.SourceSessionID = m.ID
				ns.CWD = m.CWD
				ns.StartedAt = parseTime(m.Timestamp)
			}
		case "turn_context":
			var m struct {
				Model string `json:"model"`
			}
			if json.Unmarshal(rec.Payload, &m) == nil && m.Model != "" {
				ns.Model = m.Model
			}
		case "response_item":
			var p codexMsgPayload
			if json.Unmarshal(rec.Payload, &p) != nil || p.Type != "message" {
				continue
			}
			if p.Role == "developer" || p.Role == "system" {
				continue
			}
			var blocks []Block
			for _, c := range p.Content {
				if strings.TrimSpace(c.Text) == "" {
					continue
				}
				if p.Role == "user" && isCodexSystemInjection(c.Text) {
					continue
				}
				blocks = append(blocks, Block{Type: "text", Text: c.Text})
			}
			if len(blocks) == 0 {
				continue
			}
			role := RoleUser
			if p.Role == "assistant" {
				role = RoleAssistant
			}
			ns.Messages = append(ns.Messages, Message{Role: role, Content: blocks, TS: parseTime(rec.Timestamp)})
		}
	}
	if n := len(ns.Messages); n > 0 {
		ns.EndedAt = ns.Messages[n-1].TS
	}
	return ns, sc.Err()
}
