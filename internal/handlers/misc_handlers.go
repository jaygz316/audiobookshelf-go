package handlers

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"audiobookshelf/internal/core"
	inotification "audiobookshelf/internal/notification"
)

type ApiKeyResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	UserID    string `json:"userId"`
	Username  string `json:"username"`
	ExpiresAt string `json:"expiresAt"`
	CreatedAt string `json:"createdAt"`
	IsActive  bool   `json:"isActive"`
}

type CreateApiKeyRequest struct {
	Name      string `json:"name"`
	UserID    string `json:"userId"`
	ExpiresAt string `json:"expiresAt"`
}

// handleGetApiKeys returns a list of API keys left joined with users.
func handleGetApiKeys(db *sql.DB) http.HandlerFunc {
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

		rows, err := db.Query(`
			SELECT a.id, a.name, a.userId, u.username, a.expiresAt, a.createdAt, a.isActive
			FROM apiKeys a
			LEFT JOIN users u ON a.userId = u.id
		`)
		if err != nil {
			log.Printf("[API Keys] Failed to query API keys: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		keys := make([]ApiKeyResponse, 0)
		for rows.Next() {
			var id string
			var name sql.NullString
			var userId string
			var username sql.NullString
			var expiresAt sql.NullString
			var createdAt sql.NullString
			var isActiveInt int

			if err := rows.Scan(&id, &name, &userId, &username, &expiresAt, &createdAt, &isActiveInt); err != nil {
				log.Printf("[API Keys] Failed to scan API key row: %v", err)
				http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
				return
			}

			keys = append(keys, ApiKeyResponse{
				ID:        id,
				Name:      name.String,
				UserID:    userId,
				Username:  username.String,
				ExpiresAt: expiresAt.String,
				CreatedAt: createdAt.String,
				IsActive:  isActiveInt != 0,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"apiKeys": keys,
		})
	}
}

// handlePostApiKey creates a new API key.
func handlePostApiKey(db *sql.DB) http.HandlerFunc {
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

		var req CreateApiKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid request payload"}`, http.StatusBadRequest)
			return
		}

		if strings.TrimSpace(req.Name) == "" {
			http.Error(w, `{"error": "name is required"}`, http.StatusBadRequest)
			return
		}

		if req.UserID == "" {
			http.Error(w, `{"error": "userId is required"}`, http.StatusBadRequest)
			return
		}

		// Check if the user exists and retrieve their username
		var username string
		err := db.QueryRow("SELECT username FROM users WHERE id = ?", req.UserID).Scan(&username)
		if err == sql.ErrNoRows {
			http.Error(w, `{"error": "User does not exist"}`, http.StatusBadRequest)
			return
		} else if err != nil {
			log.Printf("[API Keys] Failed to query user: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		// Generate a secure random hex API key token (48 hex characters using crypto/rand)
		tokenBytes := make([]byte, 24)
		if _, err := rand.Read(tokenBytes); err != nil {
			log.Printf("[API Keys] Failed to generate secure token: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}
		token := hex.EncodeToString(tokenBytes)

		createdAtStr := time.Now().UTC().Format(time.RFC3339)
		isActiveVal := 1

		// Insert into apiKeys table
		_, err = db.Exec(`
			INSERT INTO apiKeys (id, name, userId, expiresAt, createdAt, isActive)
			VALUES (?, ?, ?, ?, ?, ?)
		`, token, req.Name, req.UserID, req.ExpiresAt, createdAtStr, isActiveVal)
		if err != nil {
			log.Printf("[API Keys] Failed to insert API key: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"apiKey": map[string]interface{}{
				"id":        token,
				"name":      req.Name,
				"isActive":  true,
				"expiresAt": req.ExpiresAt,
				"userId":    req.UserID,
				"username":  username,
				"createdAt": createdAtStr,
				"token":     token,
			},
		})
	}
}

// handleDeleteApiKey deletes an API key.
func handleDeleteApiKey(db *sql.DB) http.HandlerFunc {
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

		id := trimPathPrefix(r.URL.Path, "/api/api-keys/")
		id = strings.TrimSuffix(id, "/")
		if id == "" {
			http.Error(w, `{"error": "ID is required"}`, http.StatusBadRequest)
			return
		}

		_, err := db.Exec("DELETE FROM apiKeys WHERE id = ?", id)
		if err != nil {
			log.Printf("[API Keys] Failed to delete API key %s: %v", id, err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

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
			log.Printf("[Notifications] Failed to query notification settings: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		if err == nil {
			if err := json.Unmarshal([]byte(valStr), &settings); err != nil {
				log.Printf("[Notifications] Failed to parse settings JSON: %v", err)
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
			log.Printf("[Notifications] Failed to load settings: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		if err == nil {
			if err := json.Unmarshal([]byte(valStr), &currentSettings); err != nil {
				log.Printf("[Notifications] Failed to unmarshal settings: %v", err)
				http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
				return
			}
		}

		// Merge payload fields into current settings
		currentBytes, err := json.Marshal(currentSettings)
		if err != nil {
			log.Printf("[Notifications] Failed to marshal current settings: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		var mergedMap map[string]interface{}
		if err := json.Unmarshal(currentBytes, &mergedMap); err != nil {
			log.Printf("[Notifications] Failed to unmarshal current settings: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		for k, v := range payload {
			mergedMap[k] = v
		}

		mergedBytes, err := json.Marshal(mergedMap)
		if err != nil {
			log.Printf("[Notifications] Failed to marshal merged map: %v", err)
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		var validatedSettings NotificationSettings
		if err := json.Unmarshal(mergedBytes, &validatedSettings); err != nil {
			log.Printf("[Notifications] Failed to unmarshal merged settings: %v", err)
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
			log.Printf("[Notifications] Failed to marshal validated settings: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		var mergedMapToSave map[string]interface{}
		if err := json.Unmarshal(cleanBytes, &mergedMapToSave); err != nil {
			log.Printf("[Notifications] Failed to unmarshal clean settings: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		if err := saveSettings(db, "notification-settings", mergedMapToSave); err != nil {
			log.Printf("[Notifications] Failed to save notification settings: %v", err)
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

// handleGetEmailsSettings returns mock email settings.
func handleGetEmailsSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"settings":{"host":"","port":465,"secure":true,"rejectUnauthorized":true,"user":"","pass":"","testAddress":"","fromAddress":"","ereaderDevices":[]}}`))
	}
}

// handleGetFeeds returns a mock list of feeds.
func handleGetFeeds(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"feeds":[]}`))
	}
}

// handleGetPlaybackSessions retrieves the list of playback sessions joined with users and media details.
func handleGetPlaybackSessions(db *sql.DB) http.HandlerFunc {
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

		userIdFilter := r.URL.Query().Get("userId")

		query := `
			SELECT
				ps.id,
				ps.userId,
				COALESCE(u.username, '') as username,
				ps.mediaItemId,
				ps.mediaItemType,
				ps.startTime,
				COALESCE(ps.updatedAt, ps.createdAt, '') as updatedAt,
				COALESCE(ps.extraData, '') as extraData,
				CASE 
					WHEN ps.mediaItemType = 'podcastEpisode' THEN COALESCE(pe.title, '')
					WHEN ps.mediaItemType = 'podcast' THEN COALESCE(p.title, '')
					ELSE COALESCE(b.title, '')
				END as title,
				CASE 
					WHEN ps.mediaItemType = 'podcastEpisode' THEN COALESCE(p.author, '')
					WHEN ps.mediaItemType = 'podcast' THEN COALESCE(p.author, '')
					ELSE COALESCE(li.authorNamesFirstLast, '')
				END as author
			FROM playbackSessions ps
			LEFT JOIN users u ON u.id = ps.userId
			LEFT JOIN books b ON b.id = ps.mediaItemId AND ps.mediaItemType = 'book'
			LEFT JOIN libraryItems li ON li.mediaId = ps.mediaItemId AND li.mediaType = 'book' AND ps.mediaItemType = 'book'
			LEFT JOIN podcastEpisodes pe ON pe.id = ps.mediaItemId AND ps.mediaItemType = 'podcastEpisode'
			LEFT JOIN podcasts p ON (p.id = pe.podcastId AND ps.mediaItemType = 'podcastEpisode') OR (p.id = ps.mediaItemId AND ps.mediaItemType = 'podcast')
		`
		var args []interface{}
		if userIdFilter != "" {
			query += " WHERE ps.userId = ?"
			args = append(args, userIdFilter)
		}
		query += " ORDER BY COALESCE(ps.updatedAt, ps.createdAt) DESC"

		rows, err := db.Query(query, args...)
		if err != nil {
			log.Printf("[Playback Sessions] Failed to query playback sessions: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type PlaybackSessionResponse struct {
			ID            string  `json:"id"`
			UserID        string  `json:"userId"`
			Username      string  `json:"username"`
			MediaItemID   string  `json:"mediaItemId"`
			MediaItemType string  `json:"mediaItemType"`
			Title         string  `json:"title"`
			Author        string  `json:"author"`
			StartTime     float64 `json:"startTime"`
			TimeListened  float64 `json:"timeListened"`
			LastTime      float64 `json:"lastTime"`
			UpdatedAt     string  `json:"updatedAt"`
			PlayMethod    string  `json:"playMethod"`
			DeviceInfo    string  `json:"deviceInfo"`
		}

		sessions := make([]PlaybackSessionResponse, 0)
		for rows.Next() {
			var id, userId, username, mediaItemId, mediaItemType, updatedAt, extraDataStr, title, author string
			var startTime float64

			err := rows.Scan(
				&id, &userId, &username, &mediaItemId, &mediaItemType, &startTime, &updatedAt, &extraDataStr, &title, &author,
			)
			if err != nil {
				log.Printf("[Playback Sessions] Failed to scan row: %v", err)
				http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
				return
			}

			// Parse extraData
			playMethod := "HLS"
			deviceInfo := "Web Client"
			timeListened := 0.0
			lastTime := 0.0

			if extraDataStr != "" {
				var extra struct {
					PlayMethod   *string  `json:"playMethod"`
					DeviceInfo   *string  `json:"deviceInfo"`
					TimeListened *float64 `json:"timeListened"`
					LastTime     *float64 `json:"lastTime"`
				}
				if err := json.Unmarshal([]byte(extraDataStr), &extra); err == nil {
					if extra.PlayMethod != nil {
						playMethod = *extra.PlayMethod
					}
					if extra.DeviceInfo != nil {
						deviceInfo = *extra.DeviceInfo
					}
					if extra.TimeListened != nil {
						timeListened = *extra.TimeListened
					}
					if extra.LastTime != nil {
						lastTime = *extra.LastTime
					}
				}
			}

			sessions = append(sessions, PlaybackSessionResponse{
				ID:            id,
				UserID:        userId,
				Username:      username,
				MediaItemID:   mediaItemId,
				MediaItemType: mediaItemType,
				Title:         title,
				Author:        author,
				StartTime:     startTime,
				TimeListened:  timeListened,
				LastTime:      lastTime,
				UpdatedAt:     updatedAt,
				PlayMethod:    playMethod,
				DeviceInfo:    deviceInfo,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"sessions": sessions,
		})
	}
}

func handleSendDefaultTestNotification(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Notifications] GET /api/notifications/test")
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
		log.Printf("[Notifications] GET /api/notifications/%s/test", id)
		
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
					log.Printf("[Notifications] Test webhook to %s failed: %v", u, err)
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
		log.Printf("[Notifications] DELETE /api/notifications/%s", id)

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
		log.Printf("[Notifications] PATCH /api/notifications/%s", id)

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

		var updatedNotif Notification
		if err := json.Unmarshal(mergedNotifBytes, &updatedNotif); err != nil {
			http.Error(w, `{"error": "Invalid fields"}`, http.StatusBadRequest)
			return
		}

		// Validate notification URLs
		for _, uStr := range updatedNotif.Urls {
			parsed, err := url.Parse(uStr)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" {
				http.Error(w, `{"error": "invalid notification url"}`, http.StatusBadRequest)
				return
			}
		}

		settings.Notifications[foundIndex] = updatedNotif

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
