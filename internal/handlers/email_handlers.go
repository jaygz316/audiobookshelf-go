package handlers

import (
	log "audiobookshelf/internal/logger"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"audiobookshelf/internal/core"
)

// handleGetEmailSettings maps to GET /api/emails/settings
func handleGetEmailSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/emails/settings")
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
		log.Infof("[Go] PATCH /api/emails/settings")
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
			log.Errorf("[Settings] Update failed: %v", err)
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
		log.Infof("[Go] POST /api/emails/test")
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
			log.Errorf("[SMTP] Test email failed: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true}`))
	}
}
