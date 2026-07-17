package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
)

func registerSettingsRoutes(mux *http.ServeMux, cfg *core.Config, db *sql.DB) {
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/settings"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetServerSettings(db)).ServeHTTP(w, r)
		} else if r.Method == http.MethodPatch {
			AuthMiddlewareWrapper(db, handleUpdateServerSettings(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/sorting-prefixes"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			AuthMiddlewareWrapper(db, handleUpdateSortingPrefixes(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/auth-settings"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetAuthSettings(db)).ServeHTTP(w, r)
		} else if r.Method == http.MethodPatch {
			AuthMiddlewareWrapper(db, handleUpdateAuthSettings(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/playback-sessions"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetPlaybackSessions(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/playback-sessions/"), func(w http.ResponseWriter, r *http.Request) {
		subPath := strings.TrimPrefix(r.URL.Path, joinPath(cfg.RouterBasePath, "/api/playback-sessions/"))
		parts := strings.Split(subPath, "/")
		if len(parts) == 1 && parts[0] != "" {
			sessionID := parts[0]
			if r.Method == http.MethodDelete {
				AuthMiddlewareWrapper(db, handleClosePlaybackSession(db, sessionID)).ServeHTTP(w, r)
				return
			}
		}
		http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/session/"), func(w http.ResponseWriter, r *http.Request) {
		subPath := strings.TrimPrefix(r.URL.Path, joinPath(cfg.RouterBasePath, "/api/session/"))
		parts := strings.Split(subPath, "/")
		if len(parts) == 1 {
			if parts[0] == "local-all" {
				if r.Method == http.MethodPost {
					AuthMiddlewareWrapper(db, handleSyncLocalSessions(db)).ServeHTTP(w, r)
					return
				}
			} else if parts[0] == "local" {
				if r.Method == http.MethodPost {
					AuthMiddlewareWrapper(db, handleSyncLocalSession(db)).ServeHTTP(w, r)
					return
				}
			}
		}
		http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
	})
}

func registerMockAndFeedRoutes(mux *http.ServeMux, cfg *core.Config, db *sql.DB) {
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/api-keys"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetApiKeys(db)).ServeHTTP(w, r)
		} else if r.Method == http.MethodPost {
			RateLimitMiddleware(LoginRateLimiter)(AuthMiddlewareWrapper(db, handlePostApiKey(db))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/api-keys/"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			AuthMiddlewareWrapper(db, handleDeleteApiKey(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/notifications"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetNotifications(db)).ServeHTTP(w, r)
		} else if r.Method == http.MethodPost || r.Method == http.MethodPatch {
			AuthMiddlewareWrapper(db, handleUpdateNotifications(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/notifications/"), handleNotificationsDispatch(db, cfg))

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/feeds"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetFeeds(db)).ServeHTTP(w, r)
		} else if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, handleCreateFeed(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/feeds/"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			AuthMiddlewareWrapper(db, handleDeleteFeed(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
}
