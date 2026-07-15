package handlers

import (
	log "audiobookshelf/internal/logger"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"audiobookshelf/internal/core"
	inotification "audiobookshelf/internal/notification"
)

type Notification = inotification.Notification
type NotificationSettings = inotification.NotificationSettings

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

func handleSendDefaultTestNotification(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Notifications] GET /api/notifications/test")
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

		fail := r.URL.Query().Get("fail") == "1"
		eventName := "onTest"
		title := "Test Notification"
		message := "This is a test notification from Audiobookshelf."
		if fail {
			title = "Test Notification (Failing)"
			message = "This test notification is sent with fail=1 to test failure handling."
		}

		// Trigger event
		inotification.TriggerEvent(r.Context(), db, eventName, nil, title, message, map[string]string{
			"isTest": "true",
			"fail":   fmt.Sprintf("%t", fail),
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}
}

func handleSendTestNotification(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := trimPathPrefix(r.URL.Path, "/api/notifications/")
		id = strings.TrimSuffix(id, "/test")
		log.Infof("[Notifications] GET /api/notifications/%s/test", id)

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

		// Load settings
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

		var targetNotif *Notification
		for _, notif := range settings.Notifications {
			if notif.ID == id {
				targetNotif = &notif
				break
			}
		}

		if targetNotif == nil {
			http.Error(w, `{"error": "Notification target not found"}`, http.StatusNotFound)
			return
		}

		// Format message
		title := "Test Notification"
		if targetNotif.TitleTemplate != "" {
			title = inotification.FormatTemplate(targetNotif.TitleTemplate, title, "Test Body", "onTest", map[string]string{"isTest": "true"})
		}
		message := "This is a test notification for target " + id
		if targetNotif.BodyTemplate != "" {
			message = inotification.FormatTemplate(targetNotif.BodyTemplate, title, message, "onTest", map[string]string{"isTest": "true"})
		}

		// Send to all URLs of this specific notification
		for _, urlStr := range targetNotif.Urls {
			notifier := inotification.NewWebhookNotifier(urlStr)
			payload := inotification.NotificationPayload{
				Title:   title,
				Message: message,
				Event:   "onTest",
				Data:    map[string]string{"isTest": "true"},
			}
			go func(u string) {
				subCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := notifier.Send(subCtx, payload); err != nil {
					log.Errorf("[Notifications] Test webhook to %s failed: %v", u, err)
				}
			}(urlStr)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}
}

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
