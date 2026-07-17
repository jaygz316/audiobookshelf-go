package notification

import (
	"context"
	"strings"
)

// NotificationPayload represents the standardized data structure sent via notifiers.
type NotificationPayload struct {
	Title   string            `json:"title"`
	Message string            `json:"message"`
	Event   string            `json:"event"`
	Data    map[string]string `json:"data,omitempty"`
}

// Notifier defines the common interface for notification dispatchers.
type Notifier interface {
	Send(ctx context.Context, payload NotificationPayload) error
}

// Notification represents a webhook target.
type Notification struct {
	ID                           string   `json:"id"`
	LibraryID                    *string  `json:"libraryId"`
	EventName                    string   `json:"eventName"`
	Urls                         []string `json:"urls"`
	TitleTemplate                string   `json:"titleTemplate"`
	BodyTemplate                 string   `json:"bodyTemplate"`
	Enabled                      bool     `json:"enabled"`
	Type                         string   `json:"type,omitempty"`
	LastFiredAt                  int64    `json:"lastFiredAt,omitempty"`
	LastAttemptFailed            bool     `json:"lastAttemptFailed,omitempty"`
	NumConsecutiveFailedAttempts int      `json:"numConsecutiveFailedAttempts,omitempty"`
	NumTimesFired                int      `json:"numTimesFired,omitempty"`
	CreatedAt                    int64    `json:"createdAt,omitempty"`
}

// NotificationSettings holds all notification targets and queue configuration.
type NotificationSettings struct {
	AppriseApiUrl        *string        `json:"appriseApiUrl"`
	MaxNotificationQueue int            `json:"maxNotificationQueue"`
	MaxFailedAttempts    int            `json:"maxFailedAttempts"`
	Notifications        []Notification `json:"notifications"`
	AppriseType          string         `json:"appriseType,omitempty"`
	NotificationDelay    int            `json:"notificationDelay,omitempty"`
}

// FormatTemplate replaces template placeholders like {{title}}, {{message}}, {{event}} and extraData keys.
func FormatTemplate(tpl string, title, message, event string, extraData map[string]string) string {
	res := tpl
	res = strings.ReplaceAll(res, "{{title}}", title)
	res = strings.ReplaceAll(res, "{{message}}", message)
	res = strings.ReplaceAll(res, "{{event}}", event)
	for k, v := range extraData {
		res = strings.ReplaceAll(res, "{{"+k+"}}", v)
	}
	return res
}
