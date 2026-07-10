package handlers

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"net/smtp"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

type EreaderDevice struct {
	Name               string   `json:"name"`
	Email              string   `json:"email"`
	AvailabilityOption string   `json:"availabilityOption"` // "adminOrUp", "specificUsers", "allUsers"
	Users              []string `json:"users"`              // User IDs
}

type EmailSettings struct {
	ID                 string          `json:"id"` // "email-settings"
	Host               string          `json:"host"`
	Port               int             `json:"port"`
	Secure             bool            `json:"secure"`
	RejectUnauthorized bool            `json:"rejectUnauthorized"`
	User               string          `json:"user"`
	Pass               string          `json:"pass"`
	TestAddress        string          `json:"testAddress"`
	FromAddress        string          `json:"fromAddress"`
	EreaderDevices     []EreaderDevice `json:"ereaderDevices"`
}

type EmailTestRequest struct {
	Host               string `json:"host"`
	Port               int    `json:"port"`
	Secure             bool   `json:"secure"`
	RejectUnauthorized bool   `json:"rejectUnauthorized"`
	User               string `json:"user"`
	Pass               string `json:"pass"`
	TestAddress        string `json:"testAddress"`
	FromAddress        string `json:"fromAddress"`
}

func defaultEmailSettings() *EmailSettings {
	return &EmailSettings{
		ID:                 "email-settings",
		Host:               "",
		Port:               587,
		Secure:             false,
		RejectUnauthorized: true,
		User:               "",
		Pass:               "",
		TestAddress:        "",
		FromAddress:        "",
		EreaderDevices:     []EreaderDevice{},
	}
}

func loadEmailSettings(db *sql.DB) (*EmailSettings, error) {
	var valStr string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'email-settings'").Scan(&valStr)
	if err != nil {
		return nil, err
	}
	var settings EmailSettings
	if err := json.Unmarshal([]byte(valStr), &settings); err != nil {
		return nil, err
	}
	return &settings, nil
}

func saveEmailSettings(db *sql.DB, settings *EmailSettings) error {
	newValBytes, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	nowStr := idb.TimeToDBStr(time.Now())
	_, err = db.Exec("INSERT INTO settings (key, value, createdAt, updatedAt) VALUES ('email-settings', ?, ?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updatedAt=excluded.updatedAt",
		string(newValBytes), nowStr, nowStr)
	return err
}

func sanitizePassword(pass string) string {
	if pass != "" {
		return "********"
	}
	return ""
}

// handleGetEmailSettings maps to GET /api/emails/settings
func handleGetEmailSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/emails/settings")
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		settings, err := loadEmailSettings(db)
		if err != nil {
			settings = defaultEmailSettings()
		}

		// Sanitize password for client response
		responseSettings := *settings
		responseSettings.Pass = sanitizePassword(settings.Pass)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(responseSettings)
	}
}

// handleUpdateEmailSettings maps to PATCH /api/emails/settings
func handleUpdateEmailSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] PATCH /api/emails/settings")
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var update EmailSettings
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		current, err := loadEmailSettings(db)
		if err != nil {
			current = defaultEmailSettings()
		}

		// Merge updates
		current.Host = update.Host
		current.Port = update.Port
		current.Secure = update.Secure
		current.RejectUnauthorized = update.RejectUnauthorized
		current.User = update.User
		current.TestAddress = update.TestAddress
		current.FromAddress = update.FromAddress

		// If a new password is provided and it is not the mask string, update it
		if update.Pass != "********" && update.Pass != "••••••••" && update.Pass != "" {
			current.Pass = update.Pass
		} else if update.Pass == "" {
			// Clear password if explicitly requested empty, but wait, usually password fields are blank on UI
			// representing "no change". To be safe, only clear if they sent an empty string and they intended to clear it.
			// In audiobookshelf, if they want no auth they clear the username too.
			if update.User == "" {
				current.Pass = ""
			}
		}

		if update.EreaderDevices != nil {
			current.EreaderDevices = update.EreaderDevices
		}

		if err := saveEmailSettings(db, current); err != nil {
			log.Printf("[Settings] Update failed: %v", err)
			http.Error(w, `{"error": "Failed to update settings"}`, http.StatusInternalServerError)
			return
		}

		responseSettings := *current
		responseSettings.Pass = sanitizePassword(current.Pass)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(responseSettings)
	}
}

// handleSendTestEmail maps to POST /api/emails/test
func handleSendTestEmail(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /api/emails/test")
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var req EmailTestRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		saved, err := loadEmailSettings(db)
		if err != nil {
			saved = defaultEmailSettings()
		}

		// Fallback to saved settings if empty
		if req.Host == "" {
			req.Host = saved.Host
			req.Port = saved.Port
			req.Secure = saved.Secure
			req.RejectUnauthorized = saved.RejectUnauthorized
			req.User = saved.User
			req.Pass = saved.Pass
			req.FromAddress = saved.FromAddress
			req.TestAddress = saved.TestAddress
		} else {
			if req.Pass == "********" || req.Pass == "••••••••" || req.Pass == "" {
				req.Pass = saved.Pass
			}
		}

		if req.Host == "" {
			http.Error(w, `{"error": "SMTP Host is required"}`, http.StatusBadRequest)
			return
		}
		if req.TestAddress == "" {
			http.Error(w, `{"error": "Test Address is required"}`, http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		subject := "Audiobookshelf Test Email"
		body := "This is a test email from your Audiobookshelf server. If you received this, your SMTP settings are configured correctly!"

		err = sendMail(ctx, req.Host, req.Port, req.Secure, req.RejectUnauthorized, req.User, req.Pass, req.FromAddress, req.TestAddress, subject, body, "", "")
		if err != nil {
			log.Printf("[SMTP] Test email failed: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true}`))
	}
}

// handleUpdateEReaderDevices maps to POST /api/emails/ereader-devices
func handleUpdateEReaderDevices(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /api/emails/ereader-devices")
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var req struct {
			EreaderDevices []EreaderDevice `json:"ereaderDevices"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		current, err := loadEmailSettings(db)
		if err != nil {
			current = defaultEmailSettings()
		}

		current.EreaderDevices = req.EreaderDevices

		if err := saveEmailSettings(db, current); err != nil {
			log.Printf("[Settings] Update ereader devices failed: %v", err)
			http.Error(w, `{"error": "Failed to update e-reader devices"}`, http.StatusInternalServerError)
			return
		}

		responseSettings := *current
		responseSettings.Pass = sanitizePassword(current.Pass)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(responseSettings)
	}
}

// handleSendEBookToDevice maps to POST /api/emails/send-ebook-to-device
func handleSendEBookToDevice(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /api/emails/send-ebook-to-device")
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		userSess := userVal.(*core.UserSession)

		var req struct {
			LibraryItemID string `json:"libraryItemId"`
			DeviceName    string `json:"deviceName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		if req.LibraryItemID == "" || req.DeviceName == "" {
			http.Error(w, `{"error": "libraryItemId and deviceName are required"}`, http.StatusBadRequest)
			return
		}

		// Load email settings
		settings, err := loadEmailSettings(db)
		if err != nil || settings.Host == "" {
			http.Error(w, `{"error": "Email settings are not configured"}`, http.StatusBadRequest)
			return
		}

		// Find the target device
		var targetDevice *EreaderDevice
		for _, dev := range settings.EreaderDevices {
			if dev.Name == req.DeviceName {
				targetDevice = &dev
				break
			}
		}
		if targetDevice == nil {
			http.Error(w, `{"error": "Device not found"}`, http.StatusNotFound)
			return
		}

		// Authorize device availability
		allowed := false
		if userSess.Type == "root" || userSess.Type == "admin" {
			allowed = true
		} else {
			switch targetDevice.AvailabilityOption {
			case "adminOrUp":
				allowed = false
			case "allUsers":
				allowed = true
			case "specificUsers":
				for _, uID := range targetDevice.Users {
					if uID == userSess.ID {
						allowed = true
						break
					}
				}
			default:
				// If not set, default to secure/admin only
				allowed = false
			}
		}

		if !allowed {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		// Fetch ebook metadata and path from database
		var mediaID, mediaType string
		err = db.QueryRow("SELECT mediaId, mediaType FROM libraryItems WHERE id = ?", req.LibraryItemID).Scan(&mediaID, &mediaType)
		if err != nil {
			http.Error(w, `{"error": "Library item not found"}`, http.StatusNotFound)
			return
		}

		if mediaType != "book" {
			http.Error(w, `{"error": "Item is not a book"}`, http.StatusBadRequest)
			return
		}

		var bTitle string
		var ebookFileBytes []byte
		err = db.QueryRow("SELECT title, ebookFile FROM books WHERE id = ?", mediaID).Scan(&bTitle, &ebookFileBytes)
		if err != nil || len(ebookFileBytes) == 0 {
			http.Error(w, `{"error": "Book has no e-book file"}`, http.StatusBadRequest)
			return
		}

		var ebook struct {
			EbookFormat string `json:"ebookFormat"`
			Metadata    struct {
				Path     string `json:"path"`
				Filename string `json:"filename"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(ebookFileBytes, &ebook); err != nil {
			http.Error(w, `{"error": "Failed to parse book ebook metadata"}`, http.StatusInternalServerError)
			return
		}

		filePath := ebook.Metadata.Path
		if filePath == "" {
			http.Error(w, `{"error": "E-book file path is not configured"}`, http.StatusBadRequest)
			return
		}

		if _, err := os.Stat(filePath); err != nil {
			log.Printf("[SMTP] Ebook file not found on disk: %s", filePath)
			http.Error(w, `{"error": "E-book file not found on server disk"}`, http.StatusNotFound)
			return
		}

		attachmentName := ebook.Metadata.Filename
		if attachmentName == "" {
			attachmentName = filepath.Base(filePath)
		}
		if attachmentName == "" || attachmentName == "." {
			ext := ".epub"
			if ebook.EbookFormat != "" {
				ext = "." + strings.TrimPrefix(ebook.EbookFormat, ".")
			}
			attachmentName = bTitle + ext
		}

		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()

		subject := fmt.Sprintf("Ebook: %s", bTitle)
		body := fmt.Sprintf("Sending e-book '%s' to your device.", bTitle)

		err = sendMail(ctx, settings.Host, settings.Port, settings.Secure, settings.RejectUnauthorized, settings.User, settings.Pass, settings.FromAddress, targetDevice.Email, subject, body, filePath, attachmentName)
		if err != nil {
			log.Printf("[SMTP] Failed to send e-book to device: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true}`))
	}
}

// sendMail connects to the SMTP server and sends a raw MIME email (with optional attachment)
func sendMail(ctx context.Context, host string, port int, secure bool, rejectUnauthorized bool, user, pass, fromAddress, toAddress, subject, body string, attachmentPath string, attachmentName string) error {
	addr := net.JoinHostPort(host, strconv.Itoa(port))

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}

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

	tlsConfig := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: !rejectUnauthorized,
	}

	if secure || port == 465 {
		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("SMTP TLS handshake failed: %w", err)
		}
		conn = tlsConn
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	if !secure && port != 465 {
		if hasStartTLS, _ := client.Extension("STARTTLS"); hasStartTLS {
			if err = client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("SMTP STARTTLS failed: %w", err)
			}
		}
	}

	if user != "" || pass != "" {
		auth := smtp.PlainAuth("", user, pass, host)
		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP auth failed: %w", err)
		}
	}

	if err = client.Mail(fromAddress); err != nil {
		return fmt.Errorf("failed to set SMTP MAIL FROM: %w", err)
	}

	if err = client.Rcpt(toAddress); err != nil {
		return fmt.Errorf("failed to set SMTP RCPT TO (%s): %w", toAddress, err)
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

	buf := new(bytes.Buffer)
	buf.WriteString(fmt.Sprintf("From: %s\r\n", fromAddress))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", toAddress))
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	buf.WriteString("MIME-Version: 1.0\r\n")

	writer := multipart.NewWriter(buf)
	if attachmentPath != "" {
		buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=%s\r\n\r\n", writer.Boundary()))

		// Text part
		if body != "" {
			partHeader := make(textproto.MIMEHeader)
			partHeader.Set("Content-Type", "text/plain; charset=UTF-8")
			part, err := writer.CreatePart(partHeader)
			if err != nil {
				return fmt.Errorf("failed to create MIME text part: %w", err)
			}
			if _, err = part.Write([]byte(body)); err != nil {
				return fmt.Errorf("failed to write body to MIME part: %w", err)
			}
		}

		// Attachment part
		attachmentBytes, err := os.ReadFile(attachmentPath)
		if err != nil {
			return fmt.Errorf("failed to read attachment file: %w", err)
		}

		partHeader := make(textproto.MIMEHeader)
		partHeader.Set("Content-Type", "application/octet-stream")
		partHeader.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, attachmentName))
		partHeader.Set("Content-Transfer-Encoding", "base64")
		part, err := writer.CreatePart(partHeader)
		if err != nil {
			return fmt.Errorf("failed to create MIME attachment part: %w", err)
		}

		b64 := base64.StdEncoding.EncodeToString(attachmentBytes)
		if _, err = part.Write([]byte(b64)); err != nil {
			return fmt.Errorf("failed to write attachment data to MIME part: %w", err)
		}
	} else {
		buf.WriteString(fmt.Sprintf("Content-Type: text/plain; charset=UTF-8\r\n\r\n%s", body))
	}

	if attachmentPath != "" {
		if err = writer.Close(); err != nil {
			return fmt.Errorf("failed to close MIME writer: %w", err)
		}
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
