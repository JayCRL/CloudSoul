// Package handoff 生成跨工具/跨机器的交接上下文。
package handoff

import (
	"context"
	"fmt"
	"strings"

	"github.com/JayCRL/CloudSoul/internal/ai"
	"github.com/JayCRL/CloudSoul/internal/sessions"
)

const systemPrompt = `你是工作交接助手。把一段 AI 编程会话压缩成简洁的交接上下文，供换机器或换工具后无缝接续。用简体中文，结构如下：
## 目标
## 当前进度
## 已改动的文件/内容
## 下一步
## 坑与注意
只输出交接文档本身，不要寒暄，也不要逐句复述原文。`

// Generate 把会话压成 handoff。有 AI client 用 AI 压缩，否则降级为元数据 + 近期对话摘要。
func Generate(ctx context.Context, client *ai.Client, ns *sessions.NeutralSession) string {
	if ns == nil || len(ns.Messages) == 0 {
		return ""
	}
	if client != nil {
		if text, err := client.Complete(ctx, systemPrompt, renderTranscript(ns, 12000)); err == nil {
			if t := strings.TrimSpace(text); t != "" {
				return t
			}
		}
	}
	return fallback(ns)
}

// renderTranscript 把消息拼成文本供 AI 压缩；超长时保留末尾（交接更关注近期）。
func renderTranscript(ns *sessions.NeutralSession, maxChars int) string {
	var b strings.Builder
	for _, m := range ns.Messages {
		var line strings.Builder
		for _, blk := range m.Content {
			switch blk.Type {
			case "text", "tool_result":
				line.WriteString(blk.Text)
			case "tool_use":
				line.WriteString("[调用工具 " + blk.Name + "]")
			}
		}
		if strings.TrimSpace(line.String()) == "" {
			continue
		}
		b.WriteString(string(m.Role))
		b.WriteString(": ")
		b.WriteString(line.String())
		b.WriteString("\n\n")
	}
	s := b.String()
	if len(s) > maxChars {
		s = "...(前文略)...\n" + s[len(s)-maxChars:]
	}
	return s
}

// fallback 无 AI 时的降级 handoff：元数据 + 最近几条对话。
func fallback(ns *sessions.NeutralSession) string {
	var b strings.Builder
	b.WriteString("## 交接（自动摘要，未接 AI）\n")
	if ns.CWD != "" {
		b.WriteString(fmt.Sprintf("- 目录：%s\n", ns.CWD))
	}
	if ns.Model != "" {
		b.WriteString(fmt.Sprintf("- 模型：%s\n", ns.Model))
	}
	b.WriteString(fmt.Sprintf("- 消息数：%d\n\n## 最近对话\n", len(ns.Messages)))
	start := 0
	if len(ns.Messages) > 6 {
		start = len(ns.Messages) - 6
	}
	for _, m := range ns.Messages[start:] {
		var line strings.Builder
		for _, blk := range m.Content {
			if blk.Type == "text" || blk.Type == "tool_result" {
				line.WriteString(blk.Text)
			}
		}
		txt := strings.TrimSpace(line.String())
		if txt == "" {
			continue
		}
		if r := []rune(txt); len(r) > 200 {
			txt = string(r[:200]) + "…"
		}
		b.WriteString(fmt.Sprintf("- **%s**：%s\n", m.Role, txt))
	}
	return b.String()
}
