// Package habits 实现分层习惯的模型与合成引擎。
package habits

import "strings"

// ScopeType 是习惯层的作用域类型。
type ScopeType string

const (
	ScopeUser      ScopeType = "user"      // 你这个人：决策风格 + 个人信息（基底）
	ScopeWorkspace ScopeType = "workspace" // 某项目通用
	ScopeTool      ScopeType = "tool"      // 某工具通用（claude-code / codex）
	ScopeModel     ScopeType = "model"     // 某模型通用（opus / gpt-5.5）
	ScopeWSTool    ScopeType = "ws_tool"   // 项目 × 工具
	ScopeWSModel   ScopeType = "ws_model"  // 项目 × 模型
)

// Layer 是一条分层习惯记录。
type Layer struct {
	ScopeType ScopeType `json:"scope_type"`
	ScopeKey  string    `json:"scope_key"`
	Content   string    `json:"content"`
}

// Target 是一次合成的目标坐标（当前项目 / 工具 / 模型）。
type Target struct {
	Workspace string
	Tool      string
	Model     string
}

type step struct {
	st  ScopeType
	key string
}

// order 给出合成顺序：从最通用到最具体，后者覆盖前者。
//
//	user → workspace → tool → model → ws_tool → ws_model
//
// 已定规则：工具/模型优先于项目（tool/model 排在 workspace 之后），
// 项目内的工具/模型特化（ws_tool / ws_model）最后生效。
func (t Target) order() []step {
	return []step{
		{ScopeUser, ""},
		{ScopeWorkspace, t.Workspace},
		{ScopeTool, t.Tool},
		{ScopeModel, t.Model},
		{ScopeWSTool, joinKey(t.Workspace, t.Tool)},
		{ScopeWSModel, joinKey(t.Workspace, t.Model)},
	}
}

func joinKey(a, b string) string {
	if a == "" || b == "" {
		return ""
	}
	return a + "|" + b
}

func indexKey(st ScopeType, key string) string {
	return string(st) + "\x00" + key
}

// Compose 按优先级顺序把相关层拼成最终习惯文本（带来源标记，分节追加）。
// 不做文本级 diff：更具体的层排在后面，模型读到后面的指令就近生效。
// 无值的维度（如无项目）自动跳过其 workspace / ws_tool / ws_model 层。
func Compose(layers []Layer, t Target) string {
	idx := make(map[string]Layer, len(layers))
	for _, l := range layers {
		idx[indexKey(l.ScopeType, l.ScopeKey)] = l
	}

	var b strings.Builder
	for _, s := range t.order() {
		if s.key == "" && s.st != ScopeUser {
			continue
		}
		l, ok := idx[indexKey(s.st, s.key)]
		if !ok || strings.TrimSpace(l.Content) == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		marker := string(s.st)
		if s.key != "" {
			marker += ":" + s.key
		}
		b.WriteString("<!-- continuum:" + marker + " -->\n")
		b.WriteString(strings.TrimRight(l.Content, "\n"))
	}
	return b.String()
}
