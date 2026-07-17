package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
)

func handleCollectionsDispatch(db *sql.DB, cfg *core.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pathWithoutPrefix := trimBasePath(r.URL.Path, cfg.RouterBasePath)
		id := strings.TrimPrefix(pathWithoutPrefix, "/api/collections/")
		if id == "" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetCollection(db, id))).ServeHTTP(w, r)
		} else if r.Method == http.MethodPatch {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleUpdateCollection(db, id))).ServeHTTP(w, r)
		} else if r.Method == http.MethodDelete {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleDeleteCollection(db, id))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	}
}
