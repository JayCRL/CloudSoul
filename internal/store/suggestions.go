package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// HabitSuggestion 是 AI 提炼的一条习惯候选。
type HabitSuggestion struct {
	ID               int64   `json:"id"`
	WorkspaceID      *int64  `json:"workspace_id,omitempty"`
	ScopeType        string  `json:"scope_type"`
	ScopeKey         string  `json:"scope_key"`
	SuggestedContent string  `json:"suggested_content"`
	Status           string  `json:"status"` // pending | accepted | rejected
}

// InsertSuggestion 写入一条 AI 习惯候选。
func (s *Store) InsertSuggestion(ctx context.Context, scopeType, scopeKey, content string, workspaceID *int64, sessionID *int64) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO habit_suggestions (workspace_id, scope_type, scope_key, suggested_content, source_session_id)
		 VALUES ($1,$2,$3,$4,$5)`, workspaceID, scopeType, scopeKey, content, sessionID)
	return err
}

// ListPendingSuggestions 列出待确认的习惯候选。
func (s *Store) ListPendingSuggestions(ctx context.Context) ([]HabitSuggestion, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, workspace_id, scope_type, scope_key, suggested_content, status
		 FROM habit_suggestions WHERE status='pending' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HabitSuggestion
	for rows.Next() {
		var hs HabitSuggestion
		if err := rows.Scan(&hs.ID, &hs.WorkspaceID, &hs.ScopeType, &hs.ScopeKey, &hs.SuggestedContent, &hs.Status); err != nil {
			return nil, err
		}
		out = append(out, hs)
	}
	return out, rows.Err()
}

// UpdateSuggestionStatus 接受或拒绝一条候选。
func (s *Store) UpdateSuggestionStatus(ctx context.Context, id int64, status string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE habit_suggestions SET status=$1 WHERE id=$2 AND status='pending'`, status, id)
	return err
}

// AcceptSuggestion 接受候选 → 写入 habit_layers。
func (s *Store) AcceptSuggestion(ctx context.Context, id int64) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var hs HabitSuggestion
	err = tx.QueryRow(ctx,
		`UPDATE habit_suggestions SET status='accepted' WHERE id=$1 AND status='pending'
		 RETURNING id, workspace_id, scope_type, scope_key, suggested_content, status`,
		id).Scan(&hs.ID, &hs.WorkspaceID, &hs.ScopeType, &hs.ScopeKey, &hs.SuggestedContent, &hs.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("suggestion not found or already processed")
	}
	if err != nil {
		return err
	}
	sk := hs.ScopeKey
	if hs.ScopeType == "user" {
		sk = "" // user 层 scope_key 固定为空。
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO habit_layers (scope_type, scope_key, content, updated_at)
		 VALUES ($1,$2,$3,now())
		 ON CONFLICT (scope_type, scope_key) DO UPDATE SET content=EXCLUDED.content, updated_at=now()`,
		hs.ScopeType, sk, hs.SuggestedContent)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
