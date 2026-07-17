package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/doyensec/safeurl"
)

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
