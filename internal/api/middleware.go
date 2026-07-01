// Package api 提供 REST handlers 与共用中间件。
package api

import (
	"crypto/subtle"
	"net/http"
)

// BearerAuth 用固定 token 校验 Authorization: Bearer <token>。REST 与 MCP 端点共用。
// 使用常量时间比较，避免时序侧信道。token 只来自 env，不入日志。
func BearerAuth(token string, next http.Handler) http.Handler {
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(got, want) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
