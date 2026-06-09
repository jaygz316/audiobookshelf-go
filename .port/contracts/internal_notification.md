# Package internal/notification

This package manages dispatching notifications (email alerts, webhook triggers).

## Go Signatures

```go
package notification

import (
	"context"
)

type NotificationPayload struct {
	Title   string            `json:"title"`
	Message string            `json:"message"`
	Event   string            `json:"event"`
	Data    map[string]string `json:"data,omitempty"`
}

type Notifier interface {
	Send(ctx context.Context, payload NotificationPayload) error
}

type EmailNotifier struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	To       []string
}

func NewEmailNotifier(host string, port int, user, pass, from string, to []string) *EmailNotifier
func (n *EmailNotifier) Send(ctx context.Context, payload NotificationPayload) error

type WebhookNotifier struct {
	URL string // Discord, Telegram, or Apprise URL
}

func NewWebhookNotifier(url string) *WebhookNotifier
func (n *WebhookNotifier) Send(ctx context.Context, payload NotificationPayload) error
```

## Behavioral Notes
- **EmailNotifier**: Connects via SMTP using username/password, and generates raw multipart email frames.
- **WebhookNotifier**: Dispatches HTTP POST requests with JSON formatted payload structures. Formats payloads appropriately if the endpoint is a known platform (e.g. Discord card structure or Telegram Markdown).
