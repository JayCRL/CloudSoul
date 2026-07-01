// Package config 从环境变量加载 Continuum 服务端配置。
package config

import (
	"fmt"
	"os"
)

// Config 是 continuum-server 的运行时配置。所有敏感值只走环境变量，不入库、不写日志。
type Config struct {
	ListenAddr  string // HTTP 监听地址，默认 :8088
	DBDSN       string // PostgreSQL DSN（必填）
	BearerToken string // REST + MCP 共用的 bearer token（必填）

	// AI endpoint：handoff 压缩 / 习惯提炼用。复用用户现有中转，全部走 env。
	AIBaseURL string
	AIToken   string
	AIModel   string
}

// Load 读取环境变量并校验必填项。
func Load() (*Config, error) {
	c := &Config{
		ListenAddr:  getenv("CONTINUUM_LISTEN_ADDR", ":8088"),
		DBDSN:       os.Getenv("CONTINUUM_DB_DSN"),
		BearerToken: os.Getenv("CONTINUUM_BEARER_TOKEN"),
		AIBaseURL:   os.Getenv("CONTINUUM_AI_BASE_URL"),
		AIToken:     os.Getenv("CONTINUUM_AI_TOKEN"),
		AIModel:     getenv("CONTINUUM_AI_MODEL", "claude-haiku-4-5-20251001"),
	}
	if c.DBDSN == "" {
		return nil, fmt.Errorf("CONTINUUM_DB_DSN is required")
	}
	if c.BearerToken == "" {
		return nil, fmt.Errorf("CONTINUUM_BEARER_TOKEN is required")
	}
	return c, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
