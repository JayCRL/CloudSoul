// Package store 封装 PostgreSQL 连接池与数据访问。
package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store 持有 pgx 连接池，作为所有 repository 的底座。
type Store struct {
	Pool *pgxpool.Pool
}

// New 建立连接池并 ping 验证可达。
func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return &Store{Pool: pool}, nil
}

// Close 释放连接池。
func (s *Store) Close() {
	if s.Pool != nil {
		s.Pool.Close()
	}
}
