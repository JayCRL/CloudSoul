package sessions

import "testing"

func TestClaudeAdapterStringContent(t *testing.T) {
	raw := `{"type":"user","message":{"role":"user","content":"hello world"},"sessionId":"s1","cwd":"/tmp","timestamp":"2026-06-18T05:30:20.667Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hi there"}],"model":"claude-opus-4-8"},"sessionId":"s1","timestamp":"2026-06-18T05:30:21.000Z"}
{"type":"summary","summary":"a chat","leafUuid":"x"}`
	ns, err := ClaudeAdapter{}.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if ns.SourceSessionID != "s1" || ns.CWD != "/tmp" {
		t.Fatalf("meta 错误: %+v", ns)
	}
	if ns.Model != "claude-opus-4-8" {
		t.Fatalf("model 错误: %s", ns.Model)
	}
	if len(ns.Messages) != 2 {
		t.Fatalf("应为 2 条消息，得 %d", len(ns.Messages))
	}
	if ns.Messages[0].Content[0].Text != "hello world" {
		t.Fatalf("user 文本错误: %+v", ns.Messages[0])
	}
	if ns.Messages[1].Role != RoleAssistant || ns.Messages[1].Content[0].Text != "hi there" {
		t.Fatalf("assistant 错误: %+v", ns.Messages[1])
	}
}

func TestClaudeAdapterBlocksAndTools(t *testing.T) {
	raw := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"let me check"},{"type":"tool_use","id":"t1","name":"Read","input":{"file":"a"}}],"model":"m"},"sessionId":"s","timestamp":"2026-06-18T05:30:21.000Z"}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"file contents"}]},"sessionId":"s","timestamp":"2026-06-18T05:30:22.000Z"}`
	ns, _ := ClaudeAdapter{}.Parse([]byte(raw))
	if len(ns.Messages) != 2 {
		t.Fatalf("应为 2 条，得 %d: %+v", len(ns.Messages), ns.Messages)
	}
	if ns.Messages[0].Content[1].Type != "tool_use" || ns.Messages[0].Content[1].Name != "Read" {
		t.Fatalf("tool_use 错误: %+v", ns.Messages[0].Content)
	}
	if ns.Messages[1].Content[0].Type != "tool_result" || ns.Messages[1].Content[0].Text != "file contents" {
		t.Fatalf("tool_result 错误: %+v", ns.Messages[1].Content)
	}
}

func TestCodexAdapterFiltersInjections(t *testing.T) {
	raw := `{"timestamp":"2026-06-18T05:30:20.589Z","type":"session_meta","payload":{"id":"019ed935","timestamp":"2026-06-18T05:30:12.994Z","cwd":"C:\\Users\\x"}}
{"timestamp":"2026-06-18T05:30:20.631Z","type":"turn_context","payload":{"model":"gpt-5.5"}}
{"timestamp":"2026-06-18T05:30:20.630Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"<permissions instructions>secret</permissions>"}]}}
{"timestamp":"2026-06-18T05:30:20.667Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>\n  <cwd>C:</cwd>\n</environment_context>"}]}}
{"timestamp":"2026-06-18T05:30:20.668Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}}
{"timestamp":"2026-06-18T05:30:25.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}}`
	ns, err := CodexAdapter{}.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if ns.SourceSessionID != "019ed935" || ns.Model != "gpt-5.5" {
		t.Fatalf("meta 错误: %+v", ns)
	}
	if len(ns.Messages) != 2 {
		t.Fatalf("过滤后应剩 2 条真实消息，得 %d: %+v", len(ns.Messages), ns.Messages)
	}
	if ns.Messages[0].Content[0].Text != "hi" {
		t.Fatalf("user 消息错误: %+v", ns.Messages[0])
	}
	if ns.Messages[1].Role != RoleAssistant || ns.Messages[1].Content[0].Text != "hello" {
		t.Fatalf("assistant 消息错误: %+v", ns.Messages[1])
	}
}
