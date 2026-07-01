package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// SaveHandoff 追加一条交接（同 workspace 最新一条即当前交接）。
func (s *Store) SaveHandoff(ctx context.Context, workspaceID int64, content string, sourceSessionID *int64, isManual bool) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO handoffs (workspace_id, content, source_session_id, is_manual) VALUES ($1,$2,$3,$4)`,
		workspaceID, content, sourceSessionID, isManual)
	return err
}

// GetLatestHandoff 返回某 workspace 的最新交接；无则返回空串。
func (s *Store) GetLatestHandoff(ctx context.Context, workspaceID int64) (string, error) {
	var content string
	err := s.Pool.QueryRow(ctx,
		`SELECT content FROM handoffs WHERE workspace_id=$1 ORDER BY created_at DESC, id DESC LIMIT 1`,
		workspaceID).Scan(&content)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return content, nil
}
