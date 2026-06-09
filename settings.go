package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// handleGetServerSettings maps to GET /api/settings (Wait, Node returns settings via other payloads, but let's have GET /api/settings just in case or for browser requests)
func handleGetServerSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/settings")
		userSess := r.Context().Value(UserContextKey).(*UserSession)
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
		userSess := r.Context().Value(UserContextKey).(*UserSession)
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
		newValBytes, err := json.Marshal(currentSettings)
		if err != nil {
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		nowStr := timeToDBStr(time.Now())
		_, err = db.Exec("INSERT INTO settings (key, value, createdAt, updatedAt) VALUES ('server-settings', ?, ?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updatedAt=excluded.updatedAt",
			string(newValBytes), nowStr, nowStr)
		if err != nil {
			log.Printf("[Settings] Update failed: %v", err)
			http.Error(w, `{"error": "Failed to update settings"}`, http.StatusInternalServerError)
			return
		}

		// Format output for browser (exclude secrets)
		browserSettings := make(map[string]interface{})
		for k, v := range currentSettings {
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
		userSess := r.Context().Value(UserContextKey).(*UserSession)
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
		prefixes := []string{}
		seen := make(map[string]bool)
		for _, p := range body.SortingPrefixes {
			trimmed := strings.ToLower(strings.TrimSpace(p))
			if trimmed != "" && !seen[trimmed] {
				seen[trimmed] = true
				prefixes = append(prefixes, trimmed)
			}
		}

		// Update server-settings
		var valStr string
		err := db.QueryRow("SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
		currentSettings := make(map[string]interface{})
		if err == nil && valStr != "" {
			json.Unmarshal([]byte(valStr), &currentSettings)
		}

		currentSettings["sortingPrefixes"] = prefixes
		newValBytes, _ := json.Marshal(currentSettings)

		nowStr := timeToDBStr(time.Now())
		_, err = db.Exec("INSERT INTO settings (key, value, createdAt, updatedAt) VALUES ('server-settings', ?, ?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updatedAt=excluded.updatedAt",
			string(newValBytes), nowStr, nowStr)
		if err != nil {
			http.Error(w, `{"error": "Internal Error"}`, http.StatusInternalServerError)
			return
		}

		// Trigger bulk recomputation of ignore prefix title columns if necessary.
		// For standalone decommission, we can run simple UPDATE statements to strip prefix on titles:
		// Go helper or direct SQLite:
		// we'll run a quick worker to update books, podcasts, and series titleIgnorePrefix.
		go recomputeIgnorePrefixes(db, prefixes)

		browserSettings := make(map[string]interface{})
		for k, v := range currentSettings {
			if k != "tokenSecret" && k != "authOpenIDClientID" && k != "authOpenIDClientSecret" &&
				k != "authOpenIDMobileRedirectURIs" && k != "authOpenIDGroupClaim" && k != "authOpenIDAdvancedPermsClaim" {
				browserSettings[k] = v
			}
		}
		browserSettings["id"] = "server-settings"

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"rowsUpdated":    0, // Will be updated asynchronously
			"serverSettings": browserSettings,
		})
	}
}

func recomputeIgnorePrefixes(db *sql.DB, prefixes []string) {
	log.Printf("[Prefix Recompute] Starting recompute with prefixes: %v", prefixes)

	// A helper to batch updates
	batchUpdate := func(table string, idCol string, titleCol string, ignoreCol string) {
		query := fmt.Sprintf("SELECT %s, %s, %s FROM %s", idCol, titleCol, ignoreCol, table)
		rows, err := db.Query(query)
		if err != nil {
			log.Printf("[Prefix Recompute] Failed to query %s: %v", table, err)
			return
		}
		defer rows.Close()

		tx, err := db.Begin()
		if err != nil {
			log.Printf("[Prefix Recompute] Failed to begin tx for %s: %v", table, err)
			return
		}

		updateQuery := fmt.Sprintf("UPDATE %s SET %s = ? WHERE %s = ?", table, ignoreCol, idCol)
		stmt, err := tx.Prepare(updateQuery)
		if err != nil {
			log.Printf("[Prefix Recompute] Failed to prepare statement for %s: %v", table, err)
			tx.Rollback()
			return
		}
		defer stmt.Close()

		for rows.Next() {
			var id, title, currentIgnore string
			if err := rows.Scan(&id, &title, &currentIgnore); err != nil {
				log.Printf("[Prefix Recompute] Failed to scan %s: %v", table, err)
				continue
			}
			newIgnore := getTitleIgnorePrefixGo(title, prefixes)
			if newIgnore != currentIgnore {
				if _, err := stmt.Exec(newIgnore, id); err != nil {
					log.Printf("[Prefix Recompute] Failed to execute update on %s for id %s: %v", table, id, err)
				}
			}
		}

		if err := rows.Err(); err != nil {
			log.Printf("[Prefix Recompute] %s query iteration error: %v", table, err)
		}

		if err := tx.Commit(); err != nil {
			log.Printf("[Prefix Recompute] Failed to commit tx for %s: %v", table, err)
		}
	}

	batchUpdate("books", "id", "title", "titleIgnorePrefix")
	batchUpdate("podcasts", "id", "title", "titleIgnorePrefix")
	batchUpdate("series", "id", "name", "nameIgnorePrefix")

	log.Printf("[Prefix Recompute] Finished")
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

// handleGetAuthSettings maps to GET /api/auth-settings
func handleGetAuthSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/auth-settings")
		userSess := r.Context().Value(UserContextKey).(*UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var valStr string
		err := db.QueryRow("SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
		currentSettings := make(map[string]interface{})
		if err == nil && valStr != "" {
			json.Unmarshal([]byte(valStr), &currentSettings)
		}

		// return full authenticationSettings including clientID/clientSecret
		authSettings := map[string]interface{}{
			"authLoginCustomMessage":             currentSettings["authLoginCustomMessage"],
			"authActiveAuthMethods":              currentSettings["authActiveAuthMethods"],
			"authOpenIDIssuerURL":                currentSettings["authOpenIDIssuerURL"],
			"authOpenIDAuthorizationURL":         currentSettings["authOpenIDAuthorizationURL"],
			"authOpenIDTokenURL":                 currentSettings["authOpenIDTokenURL"],
			"authOpenIDUserInfoURL":              currentSettings["authOpenIDUserInfoURL"],
			"authOpenIDJwksURL":                  currentSettings["authOpenIDJwksURL"],
			"authOpenIDLogoutURL":                currentSettings["authOpenIDLogoutURL"],
			"authOpenIDClientID":                 currentSettings["authOpenIDClientID"],
			"authOpenIDClientSecret":             currentSettings["authOpenIDClientSecret"],
			"authOpenIDTokenSigningAlgorithm":    currentSettings["authOpenIDTokenSigningAlgorithm"],
			"authOpenIDButtonText":               currentSettings["authOpenIDButtonText"],
			"authOpenIDAutoLaunch":               currentSettings["authOpenIDAutoLaunch"],
			"authOpenIDAutoRegister":             currentSettings["authOpenIDAutoRegister"],
			"authOpenIDMatchExistingBy":          currentSettings["authOpenIDMatchExistingBy"],
			"authOpenIDMobileRedirectURIs":       currentSettings["authOpenIDMobileRedirectURIs"],
			"authOpenIDGroupClaim":               currentSettings["authOpenIDGroupClaim"],
			"authOpenIDAdvancedPermsClaim":       currentSettings["authOpenIDAdvancedPermsClaim"],
			"authOpenIDSubfolderForRedirectURLs": currentSettings["authOpenIDSubfolderForRedirectURLs"],
			"authOpenIDSamplePermissions": map[string]interface{}{
				"download":                  true,
				"accessExplicitContent":     false,
				"accessAllLibraries":        true,
				"librariesAccessible":       []string{},
				"accessAllTags":             true,
				"itemTagsSelected":          []string{},
				"selectedTagsNotAccessible": false,
			},
		}

		// Enforce defaults if nil
		if authSettings["authActiveAuthMethods"] == nil {
			authSettings["authActiveAuthMethods"] = []string{"local"}
		}
		if authSettings["authOpenIDButtonText"] == nil {
			authSettings["authOpenIDButtonText"] = "Login with OpenId"
		}
		if authSettings["authOpenIDTokenSigningAlgorithm"] == nil {
			authSettings["authOpenIDTokenSigningAlgorithm"] = "RS256"
		}
		if authSettings["authOpenIDMobileRedirectURIs"] == nil {
			authSettings["authOpenIDMobileRedirectURIs"] = []string{"audiobookshelf://oauth"}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(authSettings)
	}
}

// handleUpdateAuthSettings maps to PATCH /api/auth-settings
func handleUpdateAuthSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] PATCH /api/auth-settings")
		userSess := r.Context().Value(UserContextKey).(*UserSession)
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

		hasUpdates := false
		allowedKeys := []string{
			"authLoginCustomMessage", "authActiveAuthMethods", "authOpenIDIssuerURL", "authOpenIDAuthorizationURL",
			"authOpenIDTokenURL", "authOpenIDUserInfoURL", "authOpenIDJwksURL", "authOpenIDLogoutURL",
			"authOpenIDClientID", "authOpenIDClientSecret", "authOpenIDTokenSigningAlgorithm",
			"authOpenIDButtonText", "authOpenIDAutoLaunch", "authOpenIDAutoRegister", "authOpenIDMatchExistingBy",
			"authOpenIDMobileRedirectURIs", "authOpenIDGroupClaim", "authOpenIDAdvancedPermsClaim", "authOpenIDSubfolderForRedirectURLs",
		}

		for _, key := range allowedKeys {
			if val, ok := settingsUpdate[key]; ok {
				currentSettings[key] = val
				hasUpdates = true
			}
		}

		if hasUpdates {
			newValBytes, err := json.Marshal(currentSettings)
			if err != nil {
				http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
				return
			}
			nowStr := timeToDBStr(time.Now())
			_, err = db.Exec("INSERT INTO settings (key, value, createdAt, updatedAt) VALUES ('server-settings', ?, ?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updatedAt=excluded.updatedAt",
				string(newValBytes), nowStr, nowStr)
			if err != nil {
				http.Error(w, `{"error": "Failed to update auth settings"}`, http.StatusInternalServerError)
				return
			}
		}

		browserSettings := make(map[string]interface{})
		for k, v := range currentSettings {
			if k != "tokenSecret" && k != "authOpenIDClientID" && k != "authOpenIDClientSecret" &&
				k != "authOpenIDMobileRedirectURIs" && k != "authOpenIDGroupClaim" && k != "authOpenIDAdvancedPermsClaim" {
				browserSettings[k] = v
			}
		}
		browserSettings["id"] = "server-settings"

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"updated":        hasUpdates,
			"serverSettings": browserSettings,
		})
	}
}

// handleGetMetadataProviders maps to GET /api/search/providers
func handleGetMetadataProviders(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/search/providers")

		// Query custom metadata providers
		rows, err := db.Query("SELECT id, name, mediaType FROM customMetadataProviders")
		var customBookProviders []map[string]interface{}
		var customPodcastProviders []map[string]interface{}
		if err != nil {
			log.Printf("[Settings] Failed to query custom metadata providers: %v", err)
		} else {
			defer rows.Close()
			for rows.Next() {
				var id, name, mediaType string
				if err := rows.Scan(&id, &name, &mediaType); err != nil {
					log.Printf("[Settings] Failed to scan custom metadata provider: %v", err)
					continue
				}
				p := map[string]interface{}{
					"value": "custom-" + id,
					"text":  name,
				}
				if mediaType == "book" {
					customBookProviders = append(customBookProviders, p)
				} else if mediaType == "podcast" {
					customPodcastProviders = append(customPodcastProviders, p)
				}
			}
			if err := rows.Err(); err != nil {
				log.Printf("[Settings] Custom metadata providers query iteration error: %v", err)
			}
		}

		providerMap := map[string]string{
			"google":          "Google Books",
			"itunes":          "iTunes",
			"openlibrary":     "Open Library",
			"fantlab":         "FantLab.ru",
			"audiobookcovers": "AudiobookCovers.com",
			"audible":         "Audible.com",
			"audnexus":        "Audnexus",
			"best":            "Best",
			"all":             "All",
		}

		formatProvider := func(p string) map[string]string {
			text := p
			if t, ok := providerMap[p]; ok {
				text = t
			}
			return map[string]string{
				"value": p,
				"text":  text,
			}
		}

		bookProvidersList := []string{"google", "openlibrary", "itunes", "audible", "fantlab", "audnexus"}
		bookCoversProvidersList := []string{"google", "openlibrary", "itunes", "audible", "fantlab", "audnexus", "audiobookcovers"}

		var booksProviders []map[string]interface{}
		for _, p := range bookProvidersList {
			m := make(map[string]interface{})
			for k, v := range formatProvider(p) {
				m[k] = v
			}
			booksProviders = append(booksProviders, m)
		}
		for _, cp := range customBookProviders {
			booksProviders = append(booksProviders, cp)
		}

		var booksCoversProviders []map[string]interface{}
		booksCoversProviders = append(booksCoversProviders, map[string]interface{}{"value": "best", "text": "Best"})
		for _, p := range bookCoversProvidersList {
			m := make(map[string]interface{})
			for k, v := range formatProvider(p) {
				m[k] = v
			}
			booksCoversProviders = append(booksCoversProviders, m)
		}
		for _, cp := range customBookProviders {
			booksCoversProviders = append(booksCoversProviders, cp)
		}
		booksCoversProviders = append(booksCoversProviders, map[string]interface{}{"value": "all", "text": "All"})

		var podcastsProviders []map[string]interface{}
		podcastsProviders = append(podcastsProviders, map[string]interface{}{"value": "itunes", "text": "iTunes"})
		for _, cp := range customPodcastProviders {
			podcastsProviders = append(podcastsProviders, cp)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"providers": map[string]interface{}{
				"books":       booksProviders,
				"booksCovers": booksCoversProviders,
				"podcasts":    podcastsProviders,
			},
		})
	}
}

// handleGetCustomMetadataProviders maps to GET /api/custom-metadata-providers
func handleGetCustomMetadataProviders(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/custom-metadata-providers")

		rows, err := db.Query("SELECT id, name, mediaType, url, authHeaderValue, extraData, createdAt, updatedAt FROM customMetadataProviders")
		if err != nil {
			http.Error(w, `{"error": "Internal Error"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var list []map[string]interface{}
		for rows.Next() {
			var id, name, mediaType, url string
			var authHeaderVal, extraData, createdAt, updatedAt sql.NullString
			if err := rows.Scan(&id, &name, &mediaType, &url, &authHeaderVal, &extraData, &createdAt, &updatedAt); err != nil {
				log.Printf("[Settings] Failed to scan custom metadata provider: %v", err)
				http.Error(w, `{"error": "Internal Error"}`, http.StatusInternalServerError)
				return
			}
			m := map[string]interface{}{
				"id":        id,
				"name":      name,
				"mediaType": mediaType,
				"url":       url,
				"slug":      "custom-" + id,
			}
			if authHeaderVal.Valid {
				m["authHeaderValue"] = authHeaderVal.String
			} else {
				m["authHeaderValue"] = nil
			}
			list = append(list, m)
		}
		if err := rows.Err(); err != nil {
			log.Printf("[Settings] Custom metadata providers query iteration error: %v", err)
			http.Error(w, `{"error": "Internal Error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"providers": list,
		})
	}
}

// handleCreateCustomMetadataProvider maps to POST /api/custom-metadata-providers
func handleCreateCustomMetadataProvider(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /api/custom-metadata-providers")

		var body struct {
			Name            string  `json:"name"`
			URL             string  `json:"url"`
			MediaType       string  `json:"mediaType"`
			AuthHeaderValue *string `json:"authHeaderValue"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error": "Invalid payload"}`, http.StatusBadRequest)
			return
		}

		if body.Name == "" || body.URL == "" || body.MediaType == "" {
			http.Error(w, `{"error": "Name, url and mediaType are required"}`, http.StatusBadRequest)
			return
		}

		id := uuid.New().String()
		nowStr := timeToDBStr(time.Now())

		var authVal interface{} = nil
		if body.AuthHeaderValue != nil && *body.AuthHeaderValue != "" {
			authVal = *body.AuthHeaderValue
		}

		_, err := db.Exec("INSERT INTO customMetadataProviders (id, name, mediaType, url, authHeaderValue, extraData, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, '{}', ?, ?)",
			id, body.Name, body.MediaType, body.URL, authVal, nowStr, nowStr)
		if err != nil {
			log.Printf("[Custom Provider] Creation failed: %v", err)
			http.Error(w, `{"error": "Failed to create custom provider"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"provider": map[string]interface{}{
				"id":              id,
				"name":            body.Name,
				"mediaType":       body.MediaType,
				"url":             body.URL,
				"authHeaderValue": authVal,
				"slug":            "custom-" + id,
			},
		})
	}
}

// handleDeleteCustomMetadataProvider maps to DELETE /api/custom-metadata-providers/:id
func handleDeleteCustomMetadataProvider(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/custom-metadata-providers/")
		log.Printf("[Go] DELETE /api/custom-metadata-providers/%s", id)

		// Delete from customMetadataProviders
		_, err := db.Exec("DELETE FROM customMetadataProviders WHERE id = ?", id)
		if err != nil {
			http.Error(w, `{"error": "Failed to delete"}`, http.StatusInternalServerError)
			return
		}

		// Fallback libraries to default providers
		slug := "custom-" + id
		_, _ = db.Exec("UPDATE libraries SET provider = 'google' WHERE provider = ? AND mediaType = 'book'", slug)
		_, _ = db.Exec("UPDATE libraries SET provider = 'itunes' WHERE provider = ? AND mediaType = 'podcast'", slug)

		w.WriteHeader(http.StatusOK)
	}
}
