package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"sync"

	"audiobookshelf/internal/core"
	log "audiobookshelf/internal/logger"
)

var notificationSettingsMu sync.Mutex

// handleUpdateNotifications updates notification settings in the database.
func handleUpdateNotifications(db *sql.DB) http.HandlerFunc {
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

		notificationSettingsMu.Lock()
		defer notificationSettingsMu.Unlock()

		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		// Load current settings from database settings table key 'notification-settings'. If none, use defaults.
		currentSettings := NotificationSettings{
			AppriseApiUrl:        nil,
			MaxNotificationQueue: 25,
			MaxFailedAttempts:    5,
			Notifications:        []Notification{},
		}

		var valStr string
		err := db.QueryRow("SELECT value FROM settings WHERE key = 'notification-settings'").Scan(&valStr)
		if err != nil && err != sql.ErrNoRows {
			log.Errorf("[Notifications] Failed to load settings: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		if err == nil {
			if err := json.Unmarshal([]byte(valStr), &currentSettings); err != nil {
				log.Errorf("[Notifications] Failed to unmarshal settings: %v", err)
				http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
				return
			}
		}

		// Merge payload fields into current settings
		currentBytes, err := json.Marshal(currentSettings)
		if err != nil {
			log.Errorf("[Notifications] Failed to marshal current settings: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		var mergedMap map[string]interface{}
		if err := json.Unmarshal(currentBytes, &mergedMap); err != nil {
			log.Errorf("[Notifications] Failed to unmarshal current settings: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		for k, v := range payload {
			mergedMap[k] = v
		}

		mergedBytes, err := json.Marshal(mergedMap)
		if err != nil {
			log.Errorf("[Notifications] Failed to marshal merged map: %v", err)
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		var validatedSettings NotificationSettings
		if err := json.Unmarshal(mergedBytes, &validatedSettings); err != nil {
			log.Errorf("[Notifications] Failed to unmarshal merged settings: %v", err)
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		// Validate merged fields (ensure queue size > 0, failed attempts > 0, notifications slice not nil)
		if validatedSettings.MaxNotificationQueue <= 0 {
			http.Error(w, `{"error": "maxNotificationQueue must be greater than 0"}`, http.StatusBadRequest)
			return
		}
		if validatedSettings.MaxFailedAttempts <= 0 {
			http.Error(w, `{"error": "maxFailedAttempts must be greater than 0"}`, http.StatusBadRequest)
			return
		}
		if validatedSettings.Notifications == nil {
			http.Error(w, `{"error": "notifications cannot be null"}`, http.StatusBadRequest)
			return
		}

		// Validate AppriseApiUrl if not null/nil
		if validatedSettings.AppriseApiUrl != nil {
			parsed, err := url.Parse(*validatedSettings.AppriseApiUrl)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				http.Error(w, `{"error": "invalid appriseApiUrl"}`, http.StatusBadRequest)
				return
			}
		}

		// Validate notification URLs
		for _, notif := range validatedSettings.Notifications {
			for _, uStr := range notif.Urls {
				parsed, err := url.Parse(uStr)
				if err != nil || parsed.Scheme == "" || parsed.Host == "" {
					http.Error(w, `{"error": "invalid notification url"}`, http.StatusBadRequest)
					return
				}
			}
		}

		// Persist the updated settings map using saveSettings
		cleanBytes, err := json.Marshal(validatedSettings)
		if err != nil {
			log.Errorf("[Notifications] Failed to marshal validated settings: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		var mergedMapToSave map[string]interface{}
		if err := json.Unmarshal(cleanBytes, &mergedMapToSave); err != nil {
			log.Errorf("[Notifications] Failed to unmarshal clean settings: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		if err := saveSettings(db, "notification-settings", mergedMapToSave); err != nil {
			log.Errorf("[Notifications] Failed to save notification settings: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := map[string]interface{}{
			"settings": validatedSettings,
		}
		_ = json.NewEncoder(w).Encode(response)
	}
}
