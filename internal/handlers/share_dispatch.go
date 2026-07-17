package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
)

func handleSharesDispatch(db *sql.DB, cfg *core.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pathWithoutPrefix := trimBasePath(r.URL.Path, cfg.RouterBasePath)
		id := strings.TrimPrefix(pathWithoutPrefix, "/api/share/mediaitem/")
		if id == "" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodDelete {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleDeleteShare(db, id))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	}
}
