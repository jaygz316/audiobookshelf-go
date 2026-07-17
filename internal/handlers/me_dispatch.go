package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
)

func handleMeDispatch(db *sql.DB, cfg *core.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subPath := strings.TrimPrefix(r.URL.Path, joinPath(cfg.RouterBasePath, "/api/me/"))
		parts := strings.Split(subPath, "/")

		if len(parts) == 1 && parts[0] == "password" {
			if r.Method == http.MethodPost {
				RateLimitMiddleware(LoginRateLimiter)(AuthMiddlewareWrapper(db, handleUpdateMePassword(db))).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 1 && parts[0] == "listening-stats" {
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, handleGetMeListeningStats(db)).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 1 && parts[0] == "listening-sessions" {
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, handleGetMeListeningSessions(db)).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 1 && parts[0] == "sessions" {
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
					handleGetUserLoginSessions(db, userSess.ID)(w, r)
				})).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 2 && parts[0] == "sessions" {
			if r.Method == http.MethodDelete {
				AuthMiddlewareWrapper(db, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
					handleDeleteUserLoginSession(db, userSess.ID, parts[1])(w, r)
				})).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 1 && parts[0] == "items-in-progress" {
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, handleGetAllLibraryItemsInProgress(db)).ServeHTTP(w, r)
				return
			}
		} else if (len(parts) == 2 || (len(parts) == 3 && parts[2] != "hide-from-continue-listening" && parts[2] != "remove-from-continue-listening")) && parts[0] == "progress" {
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, handleGetMeProgress(db)).ServeHTTP(w, r)
				return
			} else if r.Method == http.MethodPatch || r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, handleCreateUpdateMeProgress(db)).ServeHTTP(w, r)
				return
			} else if r.Method == http.MethodDelete {
				AuthMiddlewareWrapper(db, handleRemoveMeProgress(db)).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 3 && parts[0] == "progress" && (parts[2] == "hide-from-continue-listening" || parts[2] == "remove-from-continue-listening") {
			if r.Method == http.MethodGet || r.Method == http.MethodPatch {
				AuthMiddlewareWrapper(db, handleHideMeProgressFromContinueListening(db)).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 3 && parts[0] == "series" && parts[2] == "remove" {
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, handleRemoveSeriesFromContinueListening(db)).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 3 && parts[0] == "series" && parts[2] == "readd" {
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, handleReaddSeriesFromContinueListening(db)).ServeHTTP(w, r)
				return
			}
		} else if parts[0] == "item" && len(parts) >= 3 && parts[2] == "bookmark" {
			if len(parts) == 3 {
				if r.Method == http.MethodPost {
					AuthMiddlewareWrapper(db, handleMeCreateBookmark(db)).ServeHTTP(w, r)
					return
				} else if r.Method == http.MethodPatch {
					AuthMiddlewareWrapper(db, handleMeUpdateBookmark(db)).ServeHTTP(w, r)
					return
				}
			} else if len(parts) == 4 {
				if r.Method == http.MethodDelete {
					AuthMiddlewareWrapper(db, handleMeRemoveBookmark(db)).ServeHTTP(w, r)
					return
				}
			}
		} else if len(parts) == 1 && parts[0] == "sync-local-progress" {
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, handleSyncLocalProgress(db)).ServeHTTP(w, r)
				return
			}
		}

		http.NotFound(w, r)
	}
}
