package habits

import (
	"strings"
	"testing"
)

// 合成顺序：user → workspace → tool → model → ws_tool → ws_model
func TestComposeOrder(t *testing.T) {
	layers := []Layer{
		{ScopeWSTool, "lingxi|codex", "WSTOOL"},
		{ScopeUser, "", "USER"},
		{ScopeTool, "codex", "TOOL"},
		{ScopeWorkspace, "lingxi", "WS"},
		{ScopeModel, "opus", "MODEL"},
	}
	got := Compose(layers, Target{Workspace: "lingxi", Tool: "codex", Model: "opus"})

	want := []string{"USER", "WS", "TOOL", "MODEL", "WSTOOL"}
	last := -1
	for _, w := range want {
		i := strings.Index(got, w)
		if i < 0 {
			t.Fatalf("缺少 %s，输出:\n%s", w, got)
		}
		if i < last {
			t.Fatalf("%s 顺序错误，输出:\n%s", w, got)
		}
		last = i
	}
}

// 工具/模型优先于项目：tool 必须排在 workspace 之后。
func TestToolOutranksWorkspace(t *testing.T) {
	layers := []Layer{
		{ScopeWorkspace, "lingxi", "PROJECT_RULE"},
		{ScopeTool, "codex", "TOOL_RULE"},
	}
	got := Compose(layers, Target{Workspace: "lingxi", Tool: "codex", Model: "opus"})
	if strings.Index(got, "TOOL_RULE") < strings.Index(got, "PROJECT_RULE") {
		t.Fatalf("工具应排在项目之后（更优先），输出:\n%s", got)
	}
}

// 无项目时跳过 workspace 及交叉层。
func TestComposeSkipsEmptyWorkspace(t *testing.T) {
	layers := []Layer{
		{ScopeUser, "", "USER"},
		{ScopeWorkspace, "lingxi", "WS"},
		{ScopeWSTool, "lingxi|codex", "WSTOOL"},
	}
	got := Compose(layers, Target{Tool: "codex", Model: "opus"})
	if strings.Contains(got, "WS") || strings.Contains(got, "WSTOOL") {
		t.Fatalf("无项目时不应出现项目相关层，输出:\n%s", got)
	}
	if !strings.Contains(got, "USER") {
		t.Fatalf("user 层缺失，输出:\n%s", got)
	}
}

// 空内容层被跳过，且带来源标记。
func TestComposeSkipsEmptyAndMarks(t *testing.T) {
	layers := []Layer{
		{ScopeUser, "", "  "},
		{ScopeTool, "codex", "real"},
	}
	got := Compose(layers, Target{Tool: "codex"})
	if strings.Contains(got, "continuum:user") {
		t.Fatalf("空 user 层不应输出，输出:\n%s", got)
	}
	if !strings.Contains(got, "<!-- continuum:tool:codex -->") {
		t.Fatalf("缺少来源标记，输出:\n%s", got)
	}
}
