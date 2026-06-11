package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

// handleGetServerSettings maps to GET /api/settings
func handleGetServerSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/settings")
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
		log.Printf("[Go] PATCH /api/settings")
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

		// Merge updates
		for k, v := range settingsUpdate {
			currentSettings[k] = v
		}

		// Save back
		if err := saveSettings(db, "server-settings", currentSettings); err != nil {
			log.Printf("[Settings] Update failed: %v", err)
			http.Error(w, `{"error": "Failed to update settings"}`, http.StatusInternalServerError)
			return
		}

		// Format output for browser (exclude secrets)
		browserSettings := sanitizeBrowserSettings(currentSettings)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"serverSettings": browserSettings,
		})
	}
}

// handleUpdateSortingPrefixes maps to PATCH /api/sorting-prefixes
func handleUpdateSortingPrefixes(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] PATCH /api/sorting-prefixes")
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

// saveSettings saves key-value pair to the settings table
func saveSettings(db *sql.DB, key string, settings map[string]interface{}) error {
	newValBytes, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	nowStr := idb.TimeToDBStr(time.Now())
	_, err = db.Exec("INSERT INTO settings (key, value, createdAt, updatedAt) VALUES (?, ?, ?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updatedAt=excluded.updatedAt",
		key, string(newValBytes), nowStr, nowStr)
	return err
}

// sanitizeBrowserSettings removes secret configurations from settings
func sanitizeBrowserSettings(settings map[string]interface{}) map[string]interface{} {
	browserSettings := make(map[string]interface{})
	for k, v := range settings {
		if k != "tokenSecret" && k != "authOpenIDClientID" && k != "authOpenIDClientSecret" &&
			k != "authOpenIDMobileRedirectURIs" && k != "authOpenIDGroupClaim" && k != "authOpenIDAdvancedPermsClaim" {
			browserSettings[k] = v
		}
	}
	// ensure essential fields exist
	browserSettings["id"] = "server-settings"
	if browserSettings["language"] == nil || browserSettings["language"] == "" {
		browserSettings["language"] = "en-us"
	}
	if browserSettings["authActiveAuthMethods"] == nil {
		browserSettings["authActiveAuthMethods"] = []string{"local"}
	}
	return browserSettings
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

// recomputeIgnorePrefixes asynchronously updates book, podcast, and series ignore prefix title columns
func recomputeIgnorePrefixes(db *sql.DB, prefixes []string) {
	log.Printf("[Prefix Recompute] Starting recompute with prefixes: %v", prefixes)
	recomputeBooksIgnorePrefixes(db, prefixes)
	recomputePodcastsIgnorePrefixes(db, prefixes)
	recomputeSeriesIgnorePrefixes(db, prefixes)
	log.Printf("[Prefix Recompute] Finished")
}

func recomputeBooksIgnorePrefixes(db *sql.DB, prefixes []string) {
	rows, err := db.Query("SELECT id, title, titleIgnorePrefix FROM books")
	if err != nil {
		log.Printf("[Prefix Recompute] Failed to query books: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, title, currentIgnore string
		if err := rows.Scan(&id, &title, &currentIgnore); err != nil {
			log.Printf("[Prefix Recompute] Failed to scan book: %v", err)
			continue
		}
		newIgnore := getTitleIgnorePrefixGo(title, prefixes)
		if newIgnore != currentIgnore {
			db.Exec("UPDATE books SET titleIgnorePrefix = ? WHERE id = ?", newIgnore, id)
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("[Prefix Recompute] Books query iteration error: %v", err)
	}
}

func recomputePodcastsIgnorePrefixes(db *sql.DB, prefixes []string) {
	rows, err := db.Query("SELECT id, title, titleIgnorePrefix FROM podcasts")
	if err != nil {
		log.Printf("[Prefix Recompute] Failed to query podcasts: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, title, currentIgnore string
		if err := rows.Scan(&id, &title, &currentIgnore); err != nil {
			log.Printf("[Prefix Recompute] Failed to scan podcast: %v", err)
			continue
		}
		newIgnore := getTitleIgnorePrefixGo(title, prefixes)
		if newIgnore != currentIgnore {
			db.Exec("UPDATE podcasts SET titleIgnorePrefix = ? WHERE id = ?", newIgnore, id)
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("[Prefix Recompute] Podcasts query iteration error: %v", err)
	}
}

func recomputeSeriesIgnorePrefixes(db *sql.DB, prefixes []string) {
	rows, err := db.Query("SELECT id, name, nameIgnorePrefix FROM series")
	if err != nil {
		log.Printf("[Prefix Recompute] Failed to query series: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, name, currentIgnore string
		if err := rows.Scan(&id, &name, &currentIgnore); err != nil {
			log.Printf("[Prefix Recompute] Failed to scan series: %v", err)
			continue
		}
		newIgnore := getTitleIgnorePrefixGo(name, prefixes)
		if newIgnore != currentIgnore {
			db.Exec("UPDATE series SET nameIgnorePrefix = ? WHERE id = ?", newIgnore, id)
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("[Prefix Recompute] Series query iteration error: %v", err)
	}
}

func getTitleIgnorePrefixGo(title string, prefixes []string) string {
	lowerTitle := strings.ToLower(title)
	for _, p := range prefixes {
		if strings.HasPrefix(lowerTitle, p+" ") {
			return title[len(p)+1:]
		}
	}
	return title
}
