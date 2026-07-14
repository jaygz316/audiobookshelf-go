package notification

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net"
	"net/http"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/doyensec/safeurl"

	log "audiobookshelf/internal/logger"
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

// EmailNotifier connects to an SMTP server to send email notifications.
type EmailNotifier struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	To       []string
}

// NewEmailNotifier creates a new EmailNotifier.
func NewEmailNotifier(host string, port int, user, pass, from string, to []string) *EmailNotifier {
	return &EmailNotifier{
		Host:     host,
		Port:     port,
		Username: user,
		Password: pass,
		From:     from,
		To:       to,
	}
}

// Send dispatches the notification payload as a raw multipart MIME email.
func (n *EmailNotifier) Send(ctx context.Context, payload NotificationPayload) error {
	addr := net.JoinHostPort(n.Host, strconv.Itoa(n.Port))

	// PORT: Use net.Dialer with Context to respect the cancellation signal.
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()

	// Set connection deadline if context has one.
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}

	// Close connection on context cancellation to unblock pending I/O.
	if ctx.Done() != nil {
		doneChan := make(chan struct{})
		defer close(doneChan)
		go func() {
			select {
			case <-ctx.Done():
				conn.Close()
			case <-doneChan:
			}
		}()
	}

	// PORT: Perform implicit TLS handshake if connecting to standard SMTPS port 465.
	if n.Port == 465 {
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName: n.Host,
		})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("SMTP TLS handshake failed: %w", err)
		}
		conn = tlsConn
	}

	client, err := smtp.NewClient(conn, n.Host)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	// PORT: Support explicit TLS upgrade via STARTTLS for non-465 ports if the server supports it.
	if n.Port != 465 {
		if hasStartTLS, _ := client.Extension("STARTTLS"); hasStartTLS {
			tlsConfig := &tls.Config{
				ServerName: n.Host,
			}
			if err = client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("SMTP STARTTLS failed: %w", err)
			}
		}
	}

	// PORT: Perform SMTP plain auth if username/password credentials are configured.
	if n.Username != "" || n.Password != "" {
		auth := smtp.PlainAuth("", n.Username, n.Password, n.Host)
		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP auth failed: %w", err)
		}
	}

	if err = client.Mail(n.From); err != nil {
		return fmt.Errorf("failed to set SMTP MAIL FROM: %w", err)
	}

	for _, to := range n.To {
		if err = client.Rcpt(to); err != nil {
			return fmt.Errorf("failed to set SMTP RCPT TO (%s): %w", to, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to open SMTP data writer: %w", err)
	}
	var writerClosed bool
	defer func() {
		if !writerClosed {
			w.Close()
		}
	}()

	// PORT: Construct a clean raw multipart MIME email body.
	buf := new(bytes.Buffer)
	buf.WriteString(fmt.Sprintf("From: %s\r\n", n.From))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(n.To, ", ")))
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", payload.Title))
	buf.WriteString("MIME-Version: 1.0\r\n")

	writer := multipart.NewWriter(buf)
	buf.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=%s\r\n\r\n", writer.Boundary()))

	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Type", "text/plain; charset=UTF-8")
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return fmt.Errorf("failed to create MIME part: %w", err)
	}

	if _, err = part.Write([]byte(payload.Message)); err != nil {
		return fmt.Errorf("failed to write message to MIME part: %w", err)
	}

	if err = writer.Close(); err != nil {
		return fmt.Errorf("failed to close MIME writer: %w", err)
	}

	if _, err = w.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("failed to write raw email body: %w", err)
	}

	writerClosed = true
	if err = w.Close(); err != nil {
		return fmt.Errorf("failed to close SMTP data writer: %w", err)
	}

	_ = client.Quit()

	return nil
}

// WebhookNotifier dispatches webhooks to Discord, Telegram, Apprise, or general webhooks.
type WebhookNotifier struct {
	URL string
}

// NewWebhookNotifier creates a new WebhookNotifier.
func NewWebhookNotifier(url string) *WebhookNotifier {
	return &WebhookNotifier{
		URL: url,
	}
}

var safeClient *http.Client

func init() {
	// PORT: Use safeurl configuration wrapper to guard outgoing webhooks against SSRF/DNS rebinding.
	config := safeurl.GetConfigBuilder().Build()
	safeClient = safeurl.Client(config).Client
}

// Send dispatches the payload to the webhook destination using a formatted JSON request.
func (n *WebhookNotifier) Send(ctx context.Context, payload NotificationPayload) error {
	var body bytes.Buffer

	// PORT: Inspect target URL and format the JSON payload structures for specific known webhooks.
	isDiscord := strings.Contains(n.URL, "discord.com/api/webhooks") || strings.Contains(n.URL, "discordapp.com/api/webhooks")
	isTelegram := strings.Contains(n.URL, "api.telegram.org")
	isApprise := strings.Contains(n.URL, "/notify") || strings.Contains(n.URL, "apprise")

	if isDiscord {
		type discordEmbed struct {
			Title       string `json:"title,omitempty"`
			Description string `json:"description,omitempty"`
			Color       int    `json:"color,omitempty"`
		}
		type discordPayload struct {
			Embeds []discordEmbed `json:"embeds"`
		}
		dp := discordPayload{
			Embeds: []discordEmbed{
				{
					Title:       payload.Title,
					Description: payload.Message,
					Color:       3447003, // Standard Discord embed color (blue)
				},
			},
		}
		if err := json.NewEncoder(&body).Encode(dp); err != nil {
			return fmt.Errorf("failed to encode Discord payload: %w", err)
		}
	} else if isTelegram {
		type telegramPayload struct {
			Text      string `json:"text"`
			ParseMode string `json:"parse_mode,omitempty"`
		}
		tp := telegramPayload{
			Text:      fmt.Sprintf("*%s*\n%s", escapeTelegramMarkdown(payload.Title), escapeTelegramMarkdown(payload.Message)),
			ParseMode: "MarkdownV2",
		}
		if err := json.NewEncoder(&body).Encode(tp); err != nil {
			return fmt.Errorf("failed to encode Telegram payload: %w", err)
		}
	} else if isApprise {
		type apprisePayload struct {
			Title string `json:"title"`
			Body  string `json:"body"`
		}
		ap := apprisePayload{
			Title: payload.Title,
			Body:  payload.Message,
		}
		if err := json.NewEncoder(&body).Encode(ap); err != nil {
			return fmt.Errorf("failed to encode Apprise payload: %w", err)
		}
	} else {
		// PORT: Default to standard raw JSON payload mapping for generic webhooks.
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			return fmt.Errorf("failed to encode general JSON payload: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.URL, &body)
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := safeClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook responded with status: %d", resp.StatusCode)
	}

	return nil
}

// escapeTelegramMarkdown escapes special MarkdownV2 characters for Telegram payloads.
func escapeTelegramMarkdown(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch r {
		case '_', '*', '[', ']', '(', ')', '~', '`', '>', '#', '+', '-', '=', '|', '{', '}', '.', '!', '\\':
			sb.WriteRune('\\')
			sb.WriteRune(r)
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
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
