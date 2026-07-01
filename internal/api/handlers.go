package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/JayCRL/CloudSoul/internal/ai"
	"github.com/JayCRL/CloudSoul/internal/handoff"
	"github.com/JayCRL/CloudSoul/internal/habits"
	"github.com/JayCRL/CloudSoul/internal/sessions"
	"github.com/JayCRL/CloudSoul/internal/store"
)

// Handlers 持有 store 与可选的 AI client，提供 /api/* 的 REST 端点。
type Handlers struct {
	Store *store.Store
	AI    *ai.Client
}

func (h *Handlers) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/ping", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/workspaces", h.listWorkspaces)
	mux.HandleFunc("POST /api/workspaces", h.upsertWorkspace)
	mux.HandleFunc("GET /api/habits", h.getHabits)
	mux.HandleFunc("PUT /api/habits", h.setHabit)
	mux.HandleFunc("POST /api/sessions/upload", h.uploadSession)
	mux.HandleFunc("GET /api/handoff", h.getHandoff)
	mux.HandleFunc("POST /api/handoff", h.saveHandoff)
	mux.HandleFunc("GET /api/search", h.searchSessions)
	mux.HandleFunc("POST /api/suggestions/ai", h.triggerSuggest)
	mux.HandleFunc("GET /api/suggestions", h.listSuggestions)
	mux.HandleFunc("POST /api/suggestions/{id}/accept", h.acceptSuggestion)
	mux.HandleFunc("POST /api/suggestions/{id}/reject", h.rejectSuggestion)
	mux.HandleFunc("GET /api/workspaces/match", h.matchWorkspace)
	mux.HandleFunc("POST /api/workspaces/sync-git", h.syncGit)
	return mux
}

func (h *Handlers) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	ws, err := h.Store.ListWorkspaces(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	if ws == nil {
		ws = []store.Workspace{}
	}
	writeJSON(w, http.StatusOK, ws)
}

func (h *Handlers) upsertWorkspace(w http.ResponseWriter, r *http.Request) {
	var wsp store.Workspace
	if err := json.NewDecoder(r.Body).Decode(&wsp); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err))
		return
	}
	id, err := h.Store.UpsertWorkspace(r.Context(), wsp)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"id": id})
}

// resolveWorkspace 优先用显式 workspace 名，否则用 cwd 匹配。
func (h *Handlers) resolveWorkspace(r *http.Request) string {
	q := r.URL.Query()
	if ws := q.Get("workspace"); ws != "" {
		return ws
	}
	if cwd := q.Get("cwd"); cwd != "" {
		if m, _ := h.Store.MatchWorkspace(r.Context(), cwd); m != nil {
			return m.Name
		}
	}
	return ""
}

func (h *Handlers) getHabits(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	t := habits.Target{Workspace: h.resolveWorkspace(r), Tool: q.Get("tool"), Model: q.Get("model")}
	text, err := h.Store.ComposedHabits(r.Context(), t)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"habits": text})
}

func (h *Handlers) setHabit(w http.ResponseWriter, r *http.Request) {
	var l habits.Layer
	if err := json.NewDecoder(r.Body).Decode(&l); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err))
		return
	}
	if err := h.Store.UpsertHabitLayer(r.Context(), l); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type uploadReq struct {
	Tool string `json:"tool"`
	Raw  string `json:"raw"`
}

// uploadSession 规范化并归档会话；若匹配到 workspace，同步生成并保存 handoff（自动触发）。
func (h *Handlers) uploadSession(w http.ResponseWriter, r *http.Request) {
	var req uploadReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err))
		return
	}
	var ad sessions.Adapter
	switch req.Tool {
	case "codex":
		ad = sessions.CodexAdapter{}
	case "claude-code":
		ad = sessions.ClaudeAdapter{}
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown tool: " + req.Tool})
		return
	}
	ns, err := ad.Parse([]byte(req.Raw))
	if err != nil {
		writeErr(w, err)
		return
	}

	var wsID *int64
	if ns.CWD != "" {
		if ws, _ := h.Store.MatchWorkspace(r.Context(), ns.CWD); ws != nil {
			wsID = &ws.ID
		}
	}
	sid, err := h.Store.InsertSession(r.Context(), ns, wsID, []byte(req.Raw))
	if err != nil {
		writeErr(w, err)
		return
	}

	resp := map[string]any{"session_id": sid, "source_session_id": ns.SourceSessionID, "messages": len(ns.Messages)}
	if wsID != nil && len(ns.Messages) > 0 {
		content := handoff.Generate(r.Context(), h.AI, ns)
		if content != "" {
			if err := h.Store.SaveHandoff(r.Context(), *wsID, content, &sid, false); err == nil {
				resp["handoff_generated"] = true
			}
		}
		resp["workspace_id"] = *wsID
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handlers) getHandoff(w http.ResponseWriter, r *http.Request) {
	name := h.resolveWorkspace(r)
	if name == "" {
		writeJSON(w, http.StatusOK, map[string]string{"handoff": ""})
		return
	}
	ws, err := h.Store.GetWorkspaceByName(r.Context(), name)
	if err != nil {
		writeErr(w, err)
		return
	}
	if ws == nil {
		writeJSON(w, http.StatusOK, map[string]string{"handoff": ""})
		return
	}
	text, err := h.Store.GetLatestHandoff(r.Context(), ws.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"handoff": text})
}

type saveHandoffReq struct {
	Workspace string `json:"workspace"`
	Content   string `json:"content"`
}

func (h *Handlers) saveHandoff(w http.ResponseWriter, r *http.Request) {
	var req saveHandoffReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err))
		return
	}
	id, err := h.Store.UpsertWorkspace(r.Context(), store.Workspace{Name: req.Workspace})
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := h.Store.SaveHandoff(r.Context(), id, req.Content, nil, true); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handlers) searchSessions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	if query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q is required"})
		return
	}
	var wsID *int64
	if ws := h.resolveWorkspace(r); ws != "" {
		if m, _ := h.Store.GetWorkspaceByName(r.Context(), ws); m != nil {
			wsID = &m.ID
		}
	}
	results, err := h.Store.SearchSessions(r.Context(), query, wsID, q.Get("tool"), 5)
	if err != nil {
		writeErr(w, err)
		return
	}
	if results == nil {
		results = []store.SearchResult{}
	}
	writeJSON(w, http.StatusOK, results)
}

func (h *Handlers) triggerSuggest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Workspace string `json:"workspace,omitempty"`
		Tool      string `json:"tool,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	var wsID int64
	if req.Workspace != "" {
		ws, _ := h.Store.GetWorkspaceByName(r.Context(), req.Workspace)
		if ws != nil {
			wsID = ws.ID
		}
	}
	msgs, _ := h.Store.SampleRecentMessages(r.Context(), wsID, 30)
	items := ai.SuggestHabits(r.Context(), h.AI, msgs)
	var suggestions []string
	for _, it := range items {
		_ = h.Store.InsertSuggestion(r.Context(), it.ScopeType, it.ScopeKey, it.Content, nil, nil)
		suggestions = append(suggestions, it.Content)
	}
	if suggestions == nil {
		suggestions = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"suggestions": suggestions})
}

func (h *Handlers) listSuggestions(w http.ResponseWriter, r *http.Request) {
	items, err := h.Store.ListPendingSuggestions(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	if items == nil {
		items = []store.HabitSuggestion{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) acceptSuggestion(w http.ResponseWriter, r *http.Request) {
	id := 0
	if _, err := fmt.Sscanf(r.PathValue("id"), "%d", &id); err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := h.Store.AcceptSuggestion(r.Context(), int64(id)); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handlers) rejectSuggestion(w http.ResponseWriter, r *http.Request) {
	id := 0
	if _, err := fmt.Sscanf(r.PathValue("id"), "%d", &id); err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := h.Store.UpdateSuggestionStatus(r.Context(), int64(id), "rejected"); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// matchWorkspace 按 cwd 匹配项目，返回含 git_remote/branch 的完整信息（agent 开工拉代码用）。
func (h *Handlers) matchWorkspace(w http.ResponseWriter, r *http.Request) {
	cwd := r.URL.Query().Get("cwd")
	if cwd == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cwd required"})
		return
	}
	ws, err := h.Store.MatchWorkspace(r.Context(), cwd)
	if err != nil {
		writeErr(w, err)
		return
	}
	if ws == nil {
		writeJSON(w, http.StatusOK, map[string]string{})
		return
	}
	writeJSON(w, http.StatusOK, ws)
}

// syncGit 收工时更新 workspace 的 git_remote 和 git_branch。
func (h *Handlers) syncGit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CWD       string `json:"cwd"`
		GitRemote string `json:"git_remote"`
		GitBranch string `json:"git_branch"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err))
		return
	}
	ws, err := h.Store.MatchWorkspace(r.Context(), req.CWD)
	if err != nil {
		writeErr(w, err)
		return
	}
	if ws == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "no matching workspace"})
		return
	}
	ws.GitRemote = req.GitRemote
	ws.GitBranch = req.GitBranch
	if _, err := h.Store.UpsertWorkspace(r.Context(), *ws); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusInternalServerError, errBody(err))
}

func errBody(err error) map[string]string {
	return map[string]string{"error": err.Error()}
}
