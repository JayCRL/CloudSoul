package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/JayCRL/CloudSoul/internal/sessions"
)

// SearchResult 是一条搜索命中结果（含上下文）。
type SearchResult struct {
	SessionID       int64               `json:"session_id"`
	SourceSessionID string              `json:"source_session_id"`
	SourceTool      string              `json:"source_tool"`
	CWD             string              `json:"cwd,omitempty"`
	Model           string              `json:"model,omitempty"`
	Messages        []sessions.Message  `json:"messages"`
}

// SearchSessions 在 session_messages.content 中全文搜索（按 text 内容 LIKE）。
// 返回匹配的消息 + metadata。
func (s *Store) SearchSessions(ctx context.Context, query string, workspaceID *int64, tool string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}
	q := query
	rows, err := s.Pool.Query(ctx,
		`SELECT DISTINCT ON (sm.session_id) sm.session_id, ss.source_tool, ss.source_session_id, ss.cwd, ss.model, sm.role, sm.content, sm.ts
		 FROM session_messages sm
		 JOIN sessions ss ON ss.id = sm.session_id
		 WHERE sm.content::text ILIKE '%' || $1 || '%'
		   AND ($2::bigint IS NULL OR ss.workspace_id = $2)
		   AND ($3::text = '' OR ss.source_tool = $3)
		 ORDER BY sm.session_id, sm.seq
		 LIMIT $4`, q, workspaceID, tool, limit*20)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type row struct {
		sid                         int64
		tool, ssid, cwd, model, role, content string
		ts                          time.Time
	}
	var items []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.sid, &r.tool, &r.ssid, &r.cwd, &r.model, &r.role, &r.content, &r.ts); err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	if len(items) == 0 {
		return nil, nil
	}

	grouped := map[int64]*SearchResult{}
	for _, it := range items {
		m, ok := grouped[it.sid]
		if !ok {
			m = &SearchResult{SessionID: it.sid, SourceSessionID: it.ssid, SourceTool: it.tool, CWD: it.cwd, Model: it.model}
			grouped[it.sid] = m
		}
		var blocks []sessions.Block
		_ = json.Unmarshal([]byte(it.content), &blocks)
		if len(blocks) > 0 {
			m.Messages = append(m.Messages, sessions.Message{Role: sessions.Role(it.role), Content: blocks, TS: it.ts})
		}
	}
	var out []SearchResult
	for _, v := range grouped {
		out = append(out, *v)
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, rows.Err()
}

// SampleRecentMessages 抽样某 workspace 下近期消息，供 AI 提炼习惯用。
func (s *Store) SampleRecentMessages(ctx context.Context, workspaceID int64, limit int) ([]sessions.Message, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT sm.role, sm.content
		 FROM session_messages sm
		 JOIN sessions ss ON ss.id = sm.session_id
		 WHERE ss.workspace_id = $1
		 ORDER BY sm.id DESC
		 LIMIT $2`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []sessions.Message
	for rows.Next() {
		var role, content string
		if err := rows.Scan(&role, &content); err != nil {
			return nil, err
		}
		var blocks []sessions.Block
		_ = json.Unmarshal([]byte(content), &blocks)
		out = append(out, sessions.Message{Role: sessions.Role(role), Content: blocks})
	}
	return out, nil
}
