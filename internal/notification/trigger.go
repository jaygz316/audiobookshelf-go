package notification

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	log "audiobookshelf/internal/logger"
)

// TriggerEvent loads notification settings from the database and dispatches any enabled notifications matching the event name.
func TriggerEvent(ctx context.Context, dbConn *sql.DB, eventName string, libraryID *string, defaultTitle, defaultMessage string, extraData map[string]string) {
	if dbConn == nil {
		return
	}

	var valStr string
	err := dbConn.QueryRow("SELECT value FROM settings WHERE key = 'notification-settings'").Scan(&valStr)
	if err != nil {
		// No notification settings configured
		return
	}

	var settings NotificationSettings
	if err := json.Unmarshal([]byte(valStr), &settings); err != nil {
		log.Printf("[Notifications] Failed to parse settings JSON: %v", err)
		return
	}

	for i, notif := range settings.Notifications {
		if !notif.Enabled {
			continue
		}

		// Match event name (e.g. "onBackupCompleted" or "onTest")
		if notif.EventName != eventName && notif.EventName != "*" {
			continue
		}

		// Match library ID if specified
		if notif.LibraryID != nil && *notif.LibraryID != "" {
			if libraryID == nil || *libraryID != *notif.LibraryID {
				continue
			}
		}

		// Format title & body using templates if they exist
		title := defaultTitle
		if notif.TitleTemplate != "" {
			title = FormatTemplate(notif.TitleTemplate, defaultTitle, defaultMessage, eventName, extraData)
		}
		message := defaultMessage
		if notif.BodyTemplate != "" {
			message = FormatTemplate(notif.BodyTemplate, defaultTitle, defaultMessage, eventName, extraData)
		}

		// Update stats in the DB
		nowEpoch := time.Now().UnixNano() / int64(time.Millisecond)

		// Dispatch to each URL
		for _, urlStr := range notif.Urls {
			notifier := NewWebhookNotifier(urlStr)
			payload := NotificationPayload{
				Title:   title,
				Message: message,
				Event:   eventName,
				Data:    extraData,
			}

			// We launch this in a goroutine or execute synchronously.
			go func(url string, idx int) {
				subCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				err := notifier.Send(subCtx, payload)

				// Update database with status
				var currentValStr string
				var currentSettings NotificationSettings
				errDb := dbConn.QueryRow("SELECT value FROM settings WHERE key = 'notification-settings'").Scan(&currentValStr)
				if errDb == nil {
					_ = json.Unmarshal([]byte(currentValStr), &currentSettings)
					if idx < len(currentSettings.Notifications) {
						currentSettings.Notifications[idx].LastFiredAt = nowEpoch
						currentSettings.Notifications[idx].NumTimesFired++
						if err != nil {
							currentSettings.Notifications[idx].LastAttemptFailed = true
							currentSettings.Notifications[idx].NumConsecutiveFailedAttempts++
						} else {
							currentSettings.Notifications[idx].LastAttemptFailed = false
							currentSettings.Notifications[idx].NumConsecutiveFailedAttempts = 0
						}
						cleanBytes, errMarshal := json.Marshal(currentSettings)
						if errMarshal == nil {
							_, _ = dbConn.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES ('notification-settings', ?)", string(cleanBytes))
						}
					}
				}

				if err != nil {
					log.Printf("[Notifications] Failed to send webhook to %s: %v", url, err)
				} else {
					log.Printf("[Notifications] Webhook sent successfully to %s", url)
				}
			}(urlStr, i)
		}
	}
}
