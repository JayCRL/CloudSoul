// continuum-server：常驻服务端，单二进制同时提供 REST API 与 MCP server。
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/JayCRL/CloudSoul/internal/ai"
	"github.com/JayCRL/CloudSoul/internal/api"
	"github.com/JayCRL/CloudSoul/internal/config"
	"github.com/JayCRL/CloudSoul/internal/dashboard"
	appmcp "github.com/JayCRL/CloudSoul/internal/mcp"
	"github.com/JayCRL/CloudSoul/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()
	st, err := store.New(ctx, cfg.DBDSN)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	// AI client 可选：未配 CONTINUUM_AI_BASE_URL 时为 nil，handoff 生成走降级。
	aiClient := ai.New(cfg.AIBaseURL, cfg.AIToken, cfg.AIModel)

	root := http.NewServeMux()

	// /healthz —— 不认证，探活 + 检查 DB 可达
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := st.Pool.Ping(r.Context()); err != nil {
			http.Error(w, "db down", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})

	// /mcp —— MCP server（认证）
	mcpServer := appmcp.NewServer(st, aiClient)
	root.Handle("/mcp", api.BearerAuth(cfg.BearerToken, appmcp.Handler(mcpServer)))

	// /api/* —— REST（认证）
	handlers := &api.Handlers{Store: st, AI: aiClient}
	root.Handle("/api/", api.BearerAuth(cfg.BearerToken, handlers.Routes()))

	// /dashboard/* —— Web 管理面板
	dash := &dashboard.Handler{Store: st, AI: aiClient, Token: cfg.BearerToken}
	root.Handle("/dashboard/login", dash)                                          // 登录页不认证
	root.Handle("/dashboard/", api.BearerAuth(cfg.BearerToken, dash))              // 其余需认证

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           root,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("continuum-server listening on %s", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
