package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"audiobookshelf/internal/core"
	log "audiobookshelf/internal/logger"
)

// handleGetNotifications returns notification settings from the database.
func handleGetNotifications(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)
		if user.Type != "root" && user.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var settings NotificationSettings
		settings.AppriseApiUrl = nil
		settings.MaxNotificationQueue = 25
		settings.MaxFailedAttempts = 5
		settings.Notifications = []Notification{}

		var valStr string
		err := db.QueryRow("SELECT value FROM settings WHERE key = 'notification-settings'").Scan(&valStr)
		if err != nil && err != sql.ErrNoRows {
			log.Errorf("[Notifications] Failed to query notification settings: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		if err == nil {
			if err := json.Unmarshal([]byte(valStr), &settings); err != nil {
				log.Errorf("[Notifications] Failed to parse settings JSON: %v", err)
				http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
				return
			}
		}

		if settings.Notifications == nil {
			settings.Notifications = []Notification{}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := map[string]interface{}{
			"data":     nil,
			"settings": settings,
		}
		_ = json.NewEncoder(w).Encode(response)
	}
}
