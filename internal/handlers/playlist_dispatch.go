package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
)

func handlePlaylistsDispatch(db *sql.DB, cfg *core.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pathWithoutPrefix := trimBasePath(r.URL.Path, cfg.RouterBasePath)
		id := strings.TrimPrefix(pathWithoutPrefix, "/api/playlists/")
		if id == "" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetPlaylist(db, id))).ServeHTTP(w, r)
		} else if r.Method == http.MethodPatch {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleUpdatePlaylist(db, id))).ServeHTTP(w, r)
		} else if r.Method == http.MethodDelete {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleDeletePlaylist(db, id))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	}
}
