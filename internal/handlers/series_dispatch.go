package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
)

func handleSeriesDispatch(db *sql.DB, cfg *core.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] %s %s", r.Method, r.URL.Path)

		subPath := strings.TrimPrefix(r.URL.Path, joinPath(cfg.RouterBasePath, "/api/series/"))
		parts := strings.Split(subPath, "/")

		if len(parts) == 1 && parts[0] != "" {
			seriesID := parts[0]
			if r.Method == http.MethodPatch {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleUpdateSeries(db, seriesID))).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 2 && parts[1] == "auto-number" {
			seriesID := parts[0]
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleAutoNumberSeries(db, seriesID))).ServeHTTP(w, r)
				return
			}
		}

		http.NotFound(w, r)
	}
}
