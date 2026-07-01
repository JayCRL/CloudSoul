// Package api 提供 REST handlers 与共用中间件。
package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// BearerAuth 用固定 token 校验。先查 Authorization header，再查 cloudsoul_token cookie（浏览器用）。
func BearerAuth(token string, next http.Handler) http.Handler {
	want := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := extractToken(r)
		if subtle.ConstantTimeCompare([]byte(got), want) != 1 {
			// 如果是 dashboard 路径、cookie 认证失败，重定向到登录页
			if strings.HasPrefix(r.URL.Path, "/dashboard/") && r.URL.Path != "/dashboard/login" {
				http.Redirect(w, r, "/dashboard/login", http.StatusSeeOther)
				return
			}
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func extractToken(r *http.Request) string {
	// 1. Authorization header
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return h[7:]
	}
	// 2. Cookie（浏览器 dashboard）
	if c, err := r.Cookie("cloudsoul_token"); err == nil && c.Value != "" {
		return c.Value
	}
	return ""
}

// SetTokenCookie 写入 token cookie（30 天有效）。
func SetTokenCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "cloudsoul_token",
		Value:    token,
		Path:     "/dashboard",
		MaxAge:   30 * 86400,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}
