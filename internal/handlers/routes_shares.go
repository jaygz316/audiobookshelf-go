package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
)

func registerShareRoutes(mux *http.ServeMux, cfg *core.Config, db *sql.DB) {
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/share/mediaitem"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleCreateShare(db))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/share/mediaitem/"), handleSharesDispatch(db, cfg))

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/shares"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetShares(db))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/s/"), func(w http.ResponseWriter, r *http.Request) {
		RateLimitMiddleware(ShareRateLimiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pathWithoutPrefix := trimBasePath(r.URL.Path, cfg.RouterBasePath)
			subPath := strings.TrimPrefix(pathWithoutPrefix, "/api/s/")
			parts := strings.Split(subPath, "/")
			if len(parts) == 0 || parts[0] == "" {
				http.NotFound(w, r)
				return
			}
			// parts[0] is the slug
			if len(parts) == 2 {
				if parts[1] == "download" {
					if r.Method == http.MethodGet {
						handleGetPublicShareDownload(db).ServeHTTP(w, r)
						return
					}
				} else if parts[1] == "stream" {
					if r.Method == http.MethodGet {
						handleGetPublicShareStream(db).ServeHTTP(w, r)
						return
					}
				} else if parts[1] == "cover" {
					if r.Method == http.MethodGet {
						handleGetPublicShareCover(db, cfg.MetadataPath).ServeHTTP(w, r)
						return
					}
				}
			} else if len(parts) == 1 {
				if r.Method == http.MethodGet {
					handleGetPublicShare(db).ServeHTTP(w, r)
					return
				}
			}
			http.NotFound(w, r)
		})).ServeHTTP(w, r)
	})
}
