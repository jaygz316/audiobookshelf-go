package handlers

import (
	ibackup "audiobookshelf/internal/backup"
	"audiobookshelf/internal/core"
	log "audiobookshelf/internal/logger"
	"audiobookshelf/internal/watcher"
	"database/sql"
	"encoding/json"
	"net/http"
)

// handleGetServerSettings maps to GET /api/settings
func handleGetServerSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/settings")
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var valStr string
		err := db.QueryRow("SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
		if err != nil {
			valStr = "{}"
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(valStr))
	}
}

// handleUpdateServerSettings maps to PATCH /api/settings
func handleUpdateServerSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] PATCH /api/settings")
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var settingsUpdate map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&settingsUpdate); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		// Read current settings
		var valStr string
		err := db.QueryRow("SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
		currentSettings := make(map[string]interface{})
		if err == nil && valStr != "" {
			json.Unmarshal([]byte(valStr), &currentSettings)
		}

		// Retrieve old values
		oldWatch := true
		if val, ok := currentSettings["watchLibraryChanges"].(bool); ok {
			oldWatch = val
		}
		oldSortingIgnore := true
		if val, ok := currentSettings["sortingIgnorePrefix"].(bool); ok {
			oldSortingIgnore = val
		}
		oldPrefixesJSON := ""
		if val, ok := currentSettings["sortingPrefixes"]; ok {
			if b, err := json.Marshal(val); err == nil {
				oldPrefixesJSON = string(b)
			}
		}
		oldBackupSchedule := ""
		if val, ok := currentSettings["backupSchedule"].(string); ok {
			oldBackupSchedule = val
		}

		// Clean sortingPrefixes if provided in settingsUpdate
		if rawPrefs, ok := settingsUpdate["sortingPrefixes"]; ok {
			var rawSlice []string
			if slice, ok := rawPrefs.([]interface{}); ok {
				for _, v := range slice {
					if s, ok := v.(string); ok {
						rawSlice = append(rawSlice, s)
					}
				}
			} else if slice, ok := rawPrefs.([]string); ok {
				rawSlice = slice
			}
			settingsUpdate["sortingPrefixes"] = cleanSortingPrefixes(rawSlice)
		}

		// Merge updates
		for k, v := range settingsUpdate {
			currentSettings[k] = v
		}

		newWatch := true
		if val, ok := currentSettings["watchLibraryChanges"].(bool); ok {
			newWatch = val
		}
		newSortingIgnore := true
		if val, ok := currentSettings["sortingIgnorePrefix"].(bool); ok {
			newSortingIgnore = val
		}
		newPrefixesJSON := ""
		if val, ok := currentSettings["sortingPrefixes"]; ok {
			if b, err := json.Marshal(val); err == nil {
				newPrefixesJSON = string(b)
			}
		}

		// Save back
		if err := saveSettings(db, "server-settings", currentSettings); err != nil {
			log.Errorf("[Settings] Update failed: %v", err)
			http.Error(w, `{"error": "Failed to update settings"}`, http.StatusInternalServerError)
			return
		}
		InvalidateAllowIframeCache()

		if newWatch != oldWatch {
			if watcher.GlobalWatcher != nil {
				watcher.GlobalWatcher.Reload()
			}
		}

		newBackupSchedule := ""
		if val, ok := currentSettings["backupSchedule"].(string); ok {
			newBackupSchedule = val
		}
		if newBackupSchedule != oldBackupSchedule {
			if ibackup.GlobalScheduler != nil {
				ibackup.GlobalScheduler.Reload()
			}
		}

		if newSortingIgnore != oldSortingIgnore || newPrefixesJSON != oldPrefixesJSON {
			var prefixes []string
			if prefVal, ok := currentSettings["sortingPrefixes"]; ok {
				if slice, ok := prefVal.([]interface{}); ok {
					for _, v := range slice {
						if s, ok := v.(string); ok {
							prefixes = append(prefixes, s)
						}
					}
				} else if slice, ok := prefVal.([]string); ok {
					prefixes = slice
				}
			}
			go recomputeIgnorePrefixes(db, prefixes)
		}

		// Format output for browser (exclude secrets)
		browserSettings := sanitizeBrowserSettings(currentSettings)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"serverSettings": browserSettings,
		})
	}
}
