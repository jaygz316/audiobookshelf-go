package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"

	"audiobookshelf/internal/core"
)

// handleUpdateEReaderDevices maps to POST /api/emails/ereader-devices
func handleUpdateEReaderDevices(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST /api/emails/ereader-devices")
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
			log.Errorf("[Settings] Update ereader devices failed: %v", err)
			http.Error(w, `{"error": "Failed to update e-reader devices"}`, http.StatusInternalServerError)
			return
		}

		responseSettings := *current
		responseSettings.Pass = sanitizePassword(current.Pass)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(responseSettings)
	}
}

// handleGetAvailableDevices maps to GET /api/emails/devices
func handleGetAvailableDevices(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/emails/devices")
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		userSess := userVal.(*core.UserSession)

		settings, err := loadEmailSettings(db)
		if err != nil {
			// If not configured, return empty list
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
			return
		}

		var availableDevices []EreaderDevice
		for _, dev := range settings.EreaderDevices {
			allowed := false
			if userSess.Type == "root" || userSess.Type == "admin" {
				allowed = true
			} else {
				switch dev.AvailabilityOption {
				case "adminOrUp":
					allowed = false
				case "allUsers":
					allowed = true
				case "specificUsers":
					for _, uID := range dev.Users {
						if uID == userSess.ID {
							allowed = true
							break
						}
					}
				default:
					allowed = false
				}
			}
			if allowed {
				availableDevices = append(availableDevices, dev)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(availableDevices)
	}
}
