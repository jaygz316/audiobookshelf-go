package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
	"audiobookshelf/internal/utils"
)

// handleUserCRUD dispatches requests to specialized handlers based on method and path.
func handleUserCRUD(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)

		pathWithoutPrefix := utils.TrimAPIPath(r.URL.Path, "/api/users")
		if r.Method == http.MethodPost && (pathWithoutPrefix == "" || pathWithoutPrefix == "/") {
			handleUserCreate(db, w, r, userSess)
			return
		}

		subPath := strings.TrimPrefix(pathWithoutPrefix, "/")
		if subPath == "" {
			http.NotFound(w, r)
			return
		}

		parts := strings.Split(subPath, "/")
		if len(parts) == 2 && parts[1] == "listening-stats" {
			if r.Method == http.MethodGet {
				handleUserListeningStats(db, w, r, userSess, parts[0])
				return
			}
		} else if len(parts) == 2 && parts[1] == "listening-sessions" {
			if r.Method == http.MethodGet {
				handleUserListeningSessionsRoute(db, w, r, userSess, parts[0])
				return
			}
		} else if len(parts) == 2 && parts[1] == "sessions" {
			if r.Method == http.MethodGet {
				targetUserID := parts[0]
				handleGetUserLoginSessions(db, targetUserID)(w, r)
				return
			}
		} else if len(parts) == 3 && parts[1] == "sessions" {
			if r.Method == http.MethodDelete {
				targetUserID := parts[0]
				sessionID := parts[2]
				handleDeleteUserLoginSession(db, targetUserID, sessionID)(w, r)
				return
			}
		}

		targetUserID := subPath
		isUnlinkRoute := false
		if strings.HasSuffix(targetUserID, "/openid-unlink") {
			targetUserID = strings.TrimSuffix(targetUserID, "/openid-unlink")
			isUnlinkRoute = true
		}

		if isUnlinkRoute {
			if r.Method == http.MethodPatch {
				handleUserOpenIDUnlink(db, w, r, userSess, targetUserID)
				return
			}
		}

		if r.Method == http.MethodGet {
			handleUserDetails(db, w, r, userSess, targetUserID)
			return
		}

		if r.Method == http.MethodPatch {
			handleUserUpdate(db, w, r, userSess, targetUserID)
			return
		}

		if r.Method == http.MethodDelete {
			handleUserDelete(db, w, r, userSess, targetUserID)
			return
		}

		http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
	}
}
