package handlers

import (
	"audiobookshelf/internal/core"
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
)

// handleUpdateSortingPrefixes maps to PATCH /api/sorting-prefixes
func handleUpdateSortingPrefixes(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] PATCH /api/sorting-prefixes")
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var body struct {
			SortingPrefixes []string `json:"sortingPrefixes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		// Clean prefixes
		prefixes := cleanSortingPrefixes(body.SortingPrefixes)

		// Update server-settings
		var valStr string
		err := db.QueryRow("SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
		currentSettings := make(map[string]interface{})
		if err == nil && valStr != "" {
			json.Unmarshal([]byte(valStr), &currentSettings)
		}

		currentSettings["sortingPrefixes"] = prefixes

		if err := saveSettings(db, "server-settings", currentSettings); err != nil {
			http.Error(w, `{"error": "Internal Error"}`, http.StatusInternalServerError)
			return
		}

		// Trigger bulk recomputation of ignore prefix title columns if necessary.
		go recomputeIgnorePrefixes(db, prefixes)

		browserSettings := sanitizeBrowserSettings(currentSettings)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"rowsUpdated":    0, // Will be updated asynchronously
			"serverSettings": browserSettings,
		})
	}
}

// cleanSortingPrefixes normalizes and removes duplicates from sorting prefixes
func cleanSortingPrefixes(rawPrefixes []string) []string {
	prefixes := []string{}
	seen := make(map[string]bool)
	for _, p := range rawPrefixes {
		trimmed := strings.ToLower(strings.TrimSpace(p))
		if trimmed != "" && !seen[trimmed] {
			seen[trimmed] = true
			prefixes = append(prefixes, trimmed)
		}
	}
	return prefixes
}
