package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
)

func handleNotificationsDispatch(db *sql.DB, cfg *core.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subPath := strings.TrimPrefix(r.URL.Path, joinPath(cfg.RouterBasePath, "/api/notifications/"))
		subPath = strings.TrimSuffix(subPath, "/")

		// Case 1: "/api/notifications/test"
		if subPath == "test" {
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, handleSendDefaultTestNotification(db)).ServeHTTP(w, r)
			} else {
				http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			}
			return
		}

		// Case 2: "/api/notifications/{id}/test"
		if strings.HasSuffix(subPath, "/test") {
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, handleSendTestNotification(db)).ServeHTTP(w, r)
			} else {
				http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			}
			return
		}

		// Case 3: "/api/notifications/{id}"
		if subPath != "" {
			if r.Method == http.MethodPatch {
				AuthMiddlewareWrapper(db, handleUpdateNotification(db)).ServeHTTP(w, r)
			} else if r.Method == http.MethodDelete {
				AuthMiddlewareWrapper(db, handleDeleteNotification(db)).ServeHTTP(w, r)
			} else {
				http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			}
			return
		}

		http.Error(w, `{"error": "Not Found"}`, http.StatusNotFound)
	}
}
