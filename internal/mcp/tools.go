package mcp

import (
	"context"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/JayCRL/CloudSoul/internal/ai"
	"github.com/JayCRL/CloudSoul/internal/habits"
	"github.com/JayCRL/CloudSoul/internal/store"
)

type PingInput struct {
	Echo string `json:"echo,omitempty"`
}
type PingOutput struct {
	Pong string `json:"pong"`
}

type ListWorkspacesInput struct{}
type ListWorkspacesOutput struct {
	Workspaces []store.Workspace `json:"workspaces"`
}

type GetHabitsInput struct {
	Workspace string `json:"workspace,omitempty"`
	Tool      string `json:"tool,omitempty"`
	Model     string `json:"model,omitempty"`
}
type GetHabitsOutput struct {
	Habits string `json:"habits"`
}

type SetHabitInput struct {
	ScopeType string `json:"scope_type"`
	ScopeKey  string `json:"scope_key,omitempty"`
	Content   string `json:"content"`
}
type SetHabitOutput struct {
	OK bool `json:"ok"`
}

type GetHandoffInput struct {
	Workspace string `json:"workspace"`
}
type GetHandoffOutput struct {
	Handoff string `json:"handoff"`
}

type SaveHandoffInput struct {
	Workspace string `json:"workspace"`
	Content   string `json:"content"`
}
type SaveHandoffOutput struct {
	OK bool `json:"ok"`
}

type SearchSessionsInput struct {
	Query     string `json:"query"`
	Workspace string `json:"workspace,omitempty"`
	Tool      string `json:"tool,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}
type SearchSessionsOutput struct {
	Results []store.SearchResult `json:"results"`
}

type SuggestHabitsInput struct {
	Workspace string `json:"workspace,omitempty"`
}
type SuggestHabitsOutput struct {
	Suggestions []string `json:"suggestions"`
}

type ListSuggestionsInput struct{}
type ListSuggestionsOutput struct {
	Suggestions []store.HabitSuggestion `json:"suggestions"`
}

// registerTools 注册 Continuum 的 MCP 工具集。
func registerTools(s *mcpsdk.Server, st *store.Store, aiClient *ai.Client) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "ping",
		Description: "健康检查：原样返回 echo，验证 MCP 端点连通。",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, in PingInput) (*mcpsdk.CallToolResult, PingOutput, error) {
		return nil, PingOutput{Pong: in.Echo}, nil
	})

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "list_workspaces",
		Description: "列出所有已登记的项目（workspace）。",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, in ListWorkspacesInput) (*mcpsdk.CallToolResult, ListWorkspacesOutput, error) {
		ws, err := st.ListWorkspaces(ctx)
		if err != nil {
			return nil, ListWorkspacesOutput{}, err
		}
		return nil, ListWorkspacesOutput{Workspaces: ws}, nil
	})

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "get_habits",
		Description: "按 项目/工具/模型 合成分层习惯（user→workspace→tool→model→ws_tool→ws_model，工具/模型优先于项目）。",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, in GetHabitsInput) (*mcpsdk.CallToolResult, GetHabitsOutput, error) {
		text, err := st.ComposedHabits(ctx, habits.Target{Workspace: in.Workspace, Tool: in.Tool, Model: in.Model})
		if err != nil {
			return nil, GetHabitsOutput{}, err
		}
		return nil, GetHabitsOutput{Habits: text}, nil
	})

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "set_habit",
		Description: "写入或更新一层习惯。scope_type ∈ {user,workspace,tool,model,ws_tool,ws_model}；scope_key 如 codex / opus / 项目名 / \"项目|工具\"（user 层留空）。",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, in SetHabitInput) (*mcpsdk.CallToolResult, SetHabitOutput, error) {
		err := st.UpsertHabitLayer(ctx, habits.Layer{
			ScopeType: habits.ScopeType(in.ScopeType),
			ScopeKey:  in.ScopeKey,
			Content:   in.Content,
		})
		if err != nil {
			return nil, SetHabitOutput{}, err
		}
		return nil, SetHabitOutput{OK: true}, nil
	})

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "get_handoff",
		Description: "取某项目最新的交接上下文（换机/换工具后接续用）。",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, in GetHandoffInput) (*mcpsdk.CallToolResult, GetHandoffOutput, error) {
		ws, err := st.GetWorkspaceByName(ctx, in.Workspace)
		if err != nil {
			return nil, GetHandoffOutput{}, err
		}
		if ws == nil {
			return nil, GetHandoffOutput{}, nil
		}
		text, err := st.GetLatestHandoff(ctx, ws.ID)
		if err != nil {
			return nil, GetHandoffOutput{}, err
		}
		return nil, GetHandoffOutput{Handoff: text}, nil
	})

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "save_handoff",
		Description: "手动写入某项目的交接上下文（兜底：把当前进度存成 handoff，覆盖为最新）。",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, in SaveHandoffInput) (*mcpsdk.CallToolResult, SaveHandoffOutput, error) {
		id, err := st.UpsertWorkspace(ctx, store.Workspace{Name: in.Workspace})
		if err != nil {
			return nil, SaveHandoffOutput{}, err
		}
		if err := st.SaveHandoff(ctx, id, in.Content, nil, true); err != nil {
			return nil, SaveHandoffOutput{}, err
		}
		return nil, SaveHandoffOutput{OK: true}, nil
	})

	// --- M4 工具 ---

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "search_sessions",
		Description: "全文搜索历史会话（跨项目/跨工具），按关键词查找你的对话内容。",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, in SearchSessionsInput) (*mcpsdk.CallToolResult, SearchSessionsOutput, error) {
		var wsID *int64
		if in.Workspace != "" {
			ws, _ := st.GetWorkspaceByName(ctx, in.Workspace)
			if ws != nil {
				wsID = &ws.ID
			}
		}
		results, err := st.SearchSessions(ctx, in.Query, wsID, in.Tool, in.Limit)
		if err != nil {
			return nil, SearchSessionsOutput{}, err
		}
		if results == nil {
			results = []store.SearchResult{}
		}
		return nil, SearchSessionsOutput{Results: results}, nil
	})

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "suggest_habits",
		Description: "AI 分析该项目的近期会话，提炼习惯候选（需人工确认后入库）。",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, in SuggestHabitsInput) (*mcpsdk.CallToolResult, SuggestHabitsOutput, error) {
		var wsID int64
		if in.Workspace != "" {
			ws, _ := st.GetWorkspaceByName(ctx, in.Workspace)
			if ws != nil {
				wsID = ws.ID
			}
		}
		msgs, _ := st.SampleRecentMessages(ctx, wsID, 30)
		items := ai.SuggestHabits(ctx, aiClient, msgs)
		var suggestions []string
		for _, it := range items {
			if err := st.InsertSuggestion(ctx, it.ScopeType, it.ScopeKey, it.Content, nil, nil); err == nil {
				suggestions = append(suggestions, it.Content)
			}
		}
		if suggestions == nil {
			suggestions = []string{}
		}
		return nil, SuggestHabitsOutput{Suggestions: suggestions}, nil
	})

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "list_habit_suggestions",
		Description: "列出待确认的 AI 习惯候选。",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, in ListSuggestionsInput) (*mcpsdk.CallToolResult, ListSuggestionsOutput, error) {
		items, err := st.ListPendingSuggestions(ctx)
		if err != nil {
			return nil, ListSuggestionsOutput{}, err
		}
		if items == nil {
			items = []store.HabitSuggestion{}
		}
		return nil, ListSuggestionsOutput{Suggestions: items}, nil
	})

	type AcceptSuggestionInput struct {
		ID int64 `json:"id"`
	}
	type AcceptSuggestionOutput struct {
		OK bool `json:"ok"`
	}
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "accept_habit",
		Description: "接受一条习惯候选 → 自动写入对应分层（如 user/tool/model/workspace）。",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, in AcceptSuggestionInput) (*mcpsdk.CallToolResult, AcceptSuggestionOutput, error) {
		if err := st.AcceptSuggestion(ctx, in.ID); err != nil {
			return nil, AcceptSuggestionOutput{}, err
		}
		return nil, AcceptSuggestionOutput{OK: true}, nil
	})

	type RejectSuggestionInput struct {
		ID int64 `json:"id"`
	}
	type RejectSuggestionOutput struct {
		OK bool `json:"ok"`
	}
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "reject_habit",
		Description: "拒绝一条习惯候选。",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, in RejectSuggestionInput) (*mcpsdk.CallToolResult, RejectSuggestionOutput, error) {
		if err := st.UpdateSuggestionStatus(ctx, in.ID, "rejected"); err != nil {
			return nil, RejectSuggestionOutput{}, err
		}
		return nil, RejectSuggestionOutput{OK: true}, nil
	})
}
