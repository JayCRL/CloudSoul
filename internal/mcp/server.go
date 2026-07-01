// Package mcp 封装 Continuum 的 MCP server（基于官方 modelcontextprotocol/go-sdk v1.6.x）。
package mcp

import (
	"net/http"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/JayCRL/CloudSoul/internal/ai"
	"github.com/JayCRL/CloudSoul/internal/store"
)

// NewServer 创建 Continuum 的 MCP server 并注册工具。aiClient 可选（nil 时建议类功能静默跳过）。
func NewServer(st *store.Store, aiClient *ai.Client) *mcpsdk.Server {
	s := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "continuum",
		Version: "0.1.0",
	}, nil)
	registerTools(s, st, aiClient)
	return s
}

// Handler 返回挂载到 /mcp 的 Streamable HTTP handler。
func Handler(s *mcpsdk.Server) http.Handler {
	return mcpsdk.NewStreamableHTTPHandler(func(r *http.Request) *mcpsdk.Server {
		return s
	}, nil)
}
