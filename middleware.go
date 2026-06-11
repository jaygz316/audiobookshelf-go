package main

import (
	"database/sql"
	"net/http"
	"strings"
)

// BasePathRewriteMiddleware ensures the request path starts with RouterBasePath.
func BasePathRewriteMiddleware(routerBasePath string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, routerBasePath) {
			r.URL.Path = joinPath(routerBasePath, r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}

// AuthMiddlewareWrapper wraps the standard AuthMiddleware from auth.go using the DB-derived token secret.
func AuthMiddlewareWrapper(db *sql.DB, next http.Handler) http.Handler {
	return AuthMiddleware(db, getTokenSecret(db), next)
}
