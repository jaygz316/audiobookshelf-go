package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
	log "audiobookshelf/internal/logger"
)

func handleDeleteNotification(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := trimPathPrefix(r.URL.Path, "/api/notifications/")
		id = strings.TrimSuffix(id, "/")
		log.Infof("[Notifications] DELETE /api/notifications/%s", id)

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

		var valStr string
		err := db.QueryRow("SELECT value FROM settings WHERE key = 'notification-settings'").Scan(&valStr)
		if err != nil {
			http.Error(w, `{"error": "Notification settings not found"}`, http.StatusNotFound)
			return
		}

		var settings NotificationSettings
		if err := json.Unmarshal([]byte(valStr), &settings); err != nil {
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		found := false
		newNotifications := []Notification{}
		for _, notif := range settings.Notifications {
			if notif.ID == id {
				found = true
				continue
			}
			newNotifications = append(newNotifications, notif)
		}

		if !found {
			http.Error(w, `{"error": "Notification target not found"}`, http.StatusNotFound)
			return
		}

		settings.Notifications = newNotifications

		// Save settings
		cleanBytes, err := json.Marshal(settings)
		if err != nil {
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		var mergedMapToSave map[string]interface{}
		if err := json.Unmarshal(cleanBytes, &mergedMapToSave); err != nil {
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		if err := saveSettings(db, "notification-settings", mergedMapToSave); err != nil {
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"settings": settings,
		})
	}
}
