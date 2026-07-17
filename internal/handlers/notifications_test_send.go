package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"audiobookshelf/internal/core"
	log "audiobookshelf/internal/logger"
	inotification "audiobookshelf/internal/notification"
)

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
