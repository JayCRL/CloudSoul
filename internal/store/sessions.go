package store

import (
	"context"
	"encoding/json"

	"github.com/JayCRL/CloudSoul/internal/sessions"
)

// InsertSession 幂等写入一次会话：规范化消息入 session_messages，原始 JSONL 存 raw_blob。
// 冲突（source_tool+source_session_id）时更新元数据并重刷消息，保证再次上传结果一致。
func (s *Store) InsertSession(ctx context.Context, ns *sessions.NeutralSession, workspaceID *int64, raw []byte) (int64, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var started, ended any
	if !ns.StartedAt.IsZero() {
		started = ns.StartedAt
	}
	if !ns.EndedAt.IsZero() {
		ended = ns.EndedAt
	}
	// raw_blob 是 JSONB 列，原始 JSONL 非单个合法 JSON，故编码为 JSON 字符串存档。
	rawJSON, _ := json.Marshal(string(raw))

	var sid int64
	err = tx.QueryRow(ctx,
		`INSERT INTO sessions (source_tool, source_session_id, workspace_id, cwd, model, started_at, ended_at, message_count, raw_blob)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 ON CONFLICT (source_tool, source_session_id) DO UPDATE
		   SET workspace_id=EXCLUDED.workspace_id, cwd=EXCLUDED.cwd, model=EXCLUDED.model,
		       started_at=EXCLUDED.started_at, ended_at=EXCLUDED.ended_at,
		       message_count=EXCLUDED.message_count, raw_blob=EXCLUDED.raw_blob
		 RETURNING id`,
		ns.SourceTool, ns.SourceSessionID, workspaceID, ns.CWD, ns.Model, started, ended, len(ns.Messages), string(rawJSON)).Scan(&sid)
	if err != nil {
		return 0, err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM session_messages WHERE session_id=$1`, sid); err != nil {
		return 0, err
	}
	for i, m := range ns.Messages {
		content, _ := json.Marshal(m.Content)
		var ts any
		if !m.TS.IsZero() {
			ts = m.TS
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO session_messages (session_id, seq, role, content, ts) VALUES ($1,$2,$3,$4,$5)`,
			sid, i, string(m.Role), string(content), ts); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return sid, nil
}
