package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"audiobookshelf/internal/core"
	log "audiobookshelf/internal/logger"
)

func handleUpdateNotification(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := trimPathPrefix(r.URL.Path, "/api/notifications/")
		id = strings.TrimSuffix(id, "/")
		log.Infof("[Notifications] PATCH /api/notifications/%s", id)

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

		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
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

		foundIndex := -1
		for idx, notif := range settings.Notifications {
			if notif.ID == id {
				foundIndex = idx
				break
			}
		}

		if foundIndex == -1 {
			http.Error(w, `{"error": "Notification target not found"}`, http.StatusNotFound)
			return
		}

		// Merge payload into target notification
		targetNotif := settings.Notifications[foundIndex]
		notifBytes, _ := json.Marshal(targetNotif)
		var targetMap map[string]interface{}
		_ = json.Unmarshal(notifBytes, &targetMap)

		for k, v := range payload {
			targetMap[k] = v
		}

		mergedNotifBytes, err := json.Marshal(targetMap)
		if err != nil {
			http.Error(w, `{"error": "Invalid fields"}`, http.StatusBadRequest)
			return
		}

		var validatedSettings Notification
		if err := json.Unmarshal(mergedNotifBytes, &validatedSettings); err != nil {
			http.Error(w, `{"error": "Invalid fields"}`, http.StatusBadRequest)
			return
		}

		// Validate notification URLs
		for _, uStr := range validatedSettings.Urls {
			parsed, err := url.Parse(uStr)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" {
				http.Error(w, `{"error": "invalid notification url"}`, http.StatusBadRequest)
				return
			}
		}

		settings.Notifications[foundIndex] = validatedSettings

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
