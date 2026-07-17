package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
)

func handleAuthorsDispatch(db *sql.DB, cfg *core.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] %s %s", r.Method, r.URL.Path)

		subPath := strings.TrimPrefix(r.URL.Path, joinPath(cfg.RouterBasePath, "/api/authors/"))
		parts := strings.Split(subPath, "/")

		if len(parts) == 1 && parts[0] != "" {
			authorID := parts[0]
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetAuthorByID(db, authorID))).ServeHTTP(w, r)
				return
			} else if r.Method == http.MethodPatch {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleUpdateAuthor(db, authorID))).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 2 && parts[1] == "image" {
			authorID := parts[0]
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetAuthorImage(db, cfg.MetadataPath, authorID))).ServeHTTP(w, r)
				return
			} else if r.Method == http.MethodDelete {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleDeleteAuthorImage(db, cfg, authorID))).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 2 && parts[1] == "match" {
			authorID := parts[0]
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleMatchAuthor(db, cfg, authorID))).ServeHTTP(w, r)
				return
			}
		}

		http.NotFound(w, r)
	}
}
