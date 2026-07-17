package handlers

import (
	log "audiobookshelf/internal/logger"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"audiobookshelf/internal/core"
	"audiobookshelf/internal/utils"
)

// handleSendEBookToDevice maps to POST /api/emails/send-ebook-to-device
func handleSendEBookToDevice(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST /api/emails/send-ebook-to-device")
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

		if !utils.IsSafeFilePath(db, MetadataPath, filePath) {
			log.Warnf("[SMTP] Unsafe ebook file path traversal blocked: %s", filePath)
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		if _, err := os.Stat(filePath); err != nil {
			log.Errorf("[SMTP] Ebook file not found on disk: %s", filePath)
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
			log.Errorf("[SMTP] Failed to send e-book to device: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true}`))
	}
}
