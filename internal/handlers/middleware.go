package handlers

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

// getDB returns the passed db if non-nil, otherwise falls back to the package-level globalDB.
func getDB(db *sql.DB) *sql.DB {
	if db != nil {
		return db
	}
	return GetGlobalDB()
}
