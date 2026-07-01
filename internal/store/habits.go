package store

import (
	"context"

	"github.com/JayCRL/CloudSoul/internal/habits"
)

// GetHabitLayers 返回全部习惯层（数据量小，合成时在内存按 target 过滤）。
func (s *Store) GetHabitLayers(ctx context.Context) ([]habits.Layer, error) {
	rows, err := s.Pool.Query(ctx, `SELECT scope_type, scope_key, content FROM habit_layers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []habits.Layer
	for rows.Next() {
		var l habits.Layer
		var st string
		if err := rows.Scan(&st, &l.ScopeKey, &l.Content); err != nil {
			return nil, err
		}
		l.ScopeType = habits.ScopeType(st)
		out = append(out, l)
	}
	return out, rows.Err()
}

// UpsertHabitLayer 按 (scope_type, scope_key) 插入或更新一层习惯。
func (s *Store) UpsertHabitLayer(ctx context.Context, l habits.Layer) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO habit_layers (scope_type, scope_key, content, updated_at)
		 VALUES ($1,$2,$3,now())
		 ON CONFLICT (scope_type, scope_key) DO UPDATE
		   SET content=EXCLUDED.content, updated_at=now()`,
		string(l.ScopeType), l.ScopeKey, l.Content)
	return err
}

// ComposedHabits 拉全部层并按 target 合成最终习惯文本。
func (s *Store) ComposedHabits(ctx context.Context, t habits.Target) (string, error) {
	layers, err := s.GetHabitLayers(ctx)
	if err != nil {
		return "", err
	}
	return habits.Compose(layers, t), nil
}
