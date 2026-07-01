package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Workspace 是一个项目（数据归属主干的主干层）。
type Workspace struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	PathGlobs []string `json:"path_globs"`
	GitRemote string   `json:"git_remote,omitempty"`
	GitBranch string   `json:"git_branch,omitempty"`
}

// ListWorkspaces 返回全部项目。
func (s *Store) ListWorkspaces(ctx context.Context) ([]Workspace, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, name, path_globs, coalesce(git_remote,''), coalesce(git_branch,'') FROM workspaces ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Workspace
	for rows.Next() {
		var w Workspace
		if err := rows.Scan(&w.ID, &w.Name, &w.PathGlobs, &w.GitRemote, &w.GitBranch); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// GetWorkspaceByName 按名字查项目；不存在返回 (nil, nil)。
func (s *Store) GetWorkspaceByName(ctx context.Context, name string) (*Workspace, error) {
	var w Workspace
	err := s.Pool.QueryRow(ctx,
		`SELECT id, name, path_globs, coalesce(git_remote,''), coalesce(git_branch,'') FROM workspaces WHERE name=$1`,
		name).Scan(&w.ID, &w.Name, &w.PathGlobs, &w.GitRemote, &w.GitBranch)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// UpsertWorkspace 按 name 插入或更新，返回 id。
func (s *Store) UpsertWorkspace(ctx context.Context, w Workspace) (int64, error) {
	if w.PathGlobs == nil {
		w.PathGlobs = []string{}
	}
	var id int64
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO workspaces (name, path_globs, git_remote, git_branch)
		 VALUES ($1,$2,$3,$4)
		 ON CONFLICT (name) DO UPDATE SET
		   path_globs = CASE WHEN cardinality($2::text[]) > 0 THEN EXCLUDED.path_globs ELSE workspaces.path_globs END,
		   git_remote = CASE WHEN $3 <> '' THEN EXCLUDED.git_remote ELSE workspaces.git_remote END,
		   git_branch = CASE WHEN $4 <> '' THEN EXCLUDED.git_branch ELSE workspaces.git_branch END
		 RETURNING id`,
		w.Name, w.PathGlobs, w.GitRemote, w.GitBranch).Scan(&id)
	return id, err
}

// MatchWorkspace 用 cwd 匹配 workspace：任一 path_glob 是 cwd 的子串即命中（MVP 简单匹配），命中多个取最长者。
func (s *Store) MatchWorkspace(ctx context.Context, cwd string) (*Workspace, error) {
	ws, err := s.ListWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	var best *Workspace
	bestLen := -1
	cwdNorm := strings.ToLower(filepathSlash(cwd))
	for i := range ws {
		for _, g := range ws[i].PathGlobs {
			if g == "" {
				continue
			}
			if strings.Contains(cwdNorm, strings.ToLower(filepathSlash(g))) && len(g) > bestLen {
				best = &ws[i]
				bestLen = len(g)
			}
		}
	}
	return best, nil
}

func filepathSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}
