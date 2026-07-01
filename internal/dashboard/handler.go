// Package dashboard 提供 CloudSoul 的 Web 管理面板（嵌入 Go 二进制，零构建）。
package dashboard

import (
	"embed"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/JayCRL/CloudSoul/internal/ai"
	"github.com/JayCRL/CloudSoul/internal/habits"
	"github.com/JayCRL/CloudSoul/internal/store"
)

//go:embed templates/*
var tmplFS embed.FS

var tmpl *template.Template

func init() {
	funcs := template.FuncMap{
		"truncate": func(s string, n int) string {
			r := []rune(s)
			if len(r) <= n {
				return s
			}
			return string(r[:n]) + "…"
		},
	}
	tmpl = template.Must(template.New("").Funcs(funcs).ParseFS(tmplFS, "templates/*.html"))
}

// Handler 是面板的 http.Handler（挂到 /dashboard/）。
type Handler struct {
	Store *store.Store
	AI    *ai.Client
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/dashboard")
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		path = "overview"
	}

	switch {
	case r.Method == "GET" && path == "overview":
		h.overview(w, r)
	case r.Method == "GET" && path == "workspaces":
		h.workspaces(w, r)
	case r.Method == "POST" && path == "workspaces/save":
		h.saveWorkspace(w, r)
	case r.Method == "GET" && path == "habits":
		h.habitsPage(w, r)
	case r.Method == "POST" && path == "habits/save":
		h.saveHabit(w, r)
	case r.Method == "GET" && path == "sessions":
		h.sessions(w, r)
	case r.Method == "GET" && path == "suggestions":
		h.suggestions(w, r)
	case r.Method == "POST" && path == "suggestions/trigger":
		h.triggerSuggestions(w, r)
	case r.Method == "POST" && path == "suggestions/accept":
		h.acceptSuggestion(w, r)
	case r.Method == "POST" && path == "suggestions/reject":
		h.rejectSuggestion(w, r)
	default:
		http.Redirect(w, r, "/dashboard/", http.StatusSeeOther)
	}
}

func (h *Handler) render(w http.ResponseWriter, name string, data map[string]any) {
	data["Active"] = name
	if err := tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// ---- overview ----

type recentSession struct {
	SourceSessionID, SourceTool, Model string
	MessageCount                       int
}
type handoffItem struct{ Name, Content string }

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	ws, _ := h.Store.ListWorkspaces(r.Context())
	if ws == nil {
		ws = []store.Workspace{}
	}

	data := map[string]any{
		"Workspaces":     ws,
		"WorkspaceCount": len(ws),
	}

	// 最近 5 条会话
	rows, _ := h.Store.Pool.Query(r.Context(),
		`SELECT source_session_id, source_tool, coalesce(model,''), message_count FROM sessions ORDER BY id DESC LIMIT 5`)
	var recentSessions []recentSession
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var s recentSession
			rows.Scan(&s.SourceSessionID, &s.SourceTool, &s.Model, &s.MessageCount)
			recentSessions = append(recentSessions, s)
		}
	}
	if recentSessions == nil {
		recentSessions = []recentSession{}
	}
	data["RecentSessions"] = recentSessions

	var total int
	h.Store.Pool.QueryRow(r.Context(), `SELECT count(*) FROM sessions`).Scan(&total)
	data["SessionCount"] = total

	var pending int
	h.Store.Pool.QueryRow(r.Context(), `SELECT count(*) FROM habit_suggestions WHERE status='pending'`).Scan(&pending)
	data["PendingCount"] = pending

	wsRows, _ := h.Store.Pool.Query(r.Context(),
		`SELECT w.name, h.content FROM handoffs h JOIN workspaces w ON w.id=h.workspace_id
		 WHERE h.id IN (SELECT max(id) FROM handoffs GROUP BY workspace_id) ORDER BY h.id DESC LIMIT 5`)
	var latestHandoffs []handoffItem
	if wsRows != nil {
		defer wsRows.Close()
		for wsRows.Next() {
			var it handoffItem
			wsRows.Scan(&it.Name, &it.Content)
			latestHandoffs = append(latestHandoffs, it)
		}
	}
	if latestHandoffs == nil {
		latestHandoffs = []handoffItem{}
	}
	data["LatestHandoffs"] = latestHandoffs

	h.render(w, "overview", data)
}

// ---- workspaces ----

func (h *Handler) workspaces(w http.ResponseWriter, r *http.Request) {
	ws, _ := h.Store.ListWorkspaces(r.Context())
	if ws == nil {
		ws = []store.Workspace{}
	}
	h.render(w, "workspaces", map[string]any{"Workspaces": ws})
}

func (h *Handler) saveWorkspace(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	ws := store.Workspace{
		Name:      r.FormValue("name"),
		GitRemote: r.FormValue("git_remote"),
		GitBranch: r.FormValue("git_branch"),
	}
	for _, g := range strings.Split(r.FormValue("path_globs"), ",") {
		if s := strings.TrimSpace(g); s != "" {
			ws.PathGlobs = append(ws.PathGlobs, s)
		}
	}
	h.Store.UpsertWorkspace(r.Context(), ws)
	http.Redirect(w, r, "/dashboard/workspaces", http.StatusSeeOther)
}

// ---- habits ----

func (h *Handler) habitsPage(w http.ResponseWriter, r *http.Request) {
	layers, _ := h.Store.GetHabitLayers(r.Context())
	if layers == nil {
		layers = []habits.Layer{}
	}
	h.render(w, "habits", map[string]any{"Layers": layers})
}

func (h *Handler) saveHabit(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	h.Store.UpsertHabitLayer(r.Context(), habits.Layer{
		ScopeType: habits.ScopeType(r.FormValue("scope_type")),
		ScopeKey:  r.FormValue("scope_key"),
		Content:   r.FormValue("content"),
	})
	http.Redirect(w, r, "/dashboard/habits", http.StatusSeeOther)
}

// ---- sessions ----

func (h *Handler) sessions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	data := map[string]any{"Query": q, "Workspace": r.URL.Query().Get("workspace"), "Tool": r.URL.Query().Get("tool")}
	if q == "" {
		h.render(w, "sessions", data)
		return
	}
	var wsID *int64
	if ws := r.URL.Query().Get("workspace"); ws != "" {
		if m, _ := h.Store.GetWorkspaceByName(r.Context(), ws); m != nil {
			wsID = &m.ID
		}
	}
	results, _ := h.Store.SearchSessions(r.Context(), q, wsID, r.URL.Query().Get("tool"), 10)
	data["Results"] = results
	h.render(w, "sessions", data)
}

// ---- suggestions ----

func (h *Handler) suggestions(w http.ResponseWriter, r *http.Request) {
	items, _ := h.Store.ListPendingSuggestions(r.Context())
	if items == nil {
		items = []store.HabitSuggestion{}
	}
	h.render(w, "suggestions", map[string]any{"Suggestions": items})
}

func (h *Handler) triggerSuggestions(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	var wsID int64
	if ws := r.FormValue("workspace"); ws != "" {
		if m, _ := h.Store.GetWorkspaceByName(r.Context(), ws); m != nil {
			wsID = m.ID
		}
	}
	msgs, _ := h.Store.SampleRecentMessages(r.Context(), wsID, 30)
	items := ai.SuggestHabits(r.Context(), h.AI, msgs)
	for _, it := range items {
		h.Store.InsertSuggestion(r.Context(), it.ScopeType, it.ScopeKey, it.Content, nil, nil)
	}
	http.Redirect(w, r, "/dashboard/suggestions", http.StatusSeeOther)
}

func (h *Handler) acceptSuggestion(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
	h.Store.AcceptSuggestion(r.Context(), id)
	http.Redirect(w, r, "/dashboard/suggestions", http.StatusSeeOther)
}

func (h *Handler) rejectSuggestion(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
	h.Store.UpdateSuggestionStatus(r.Context(), id, "rejected")
	http.Redirect(w, r, "/dashboard/suggestions", http.StatusSeeOther)
}
