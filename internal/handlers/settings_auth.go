package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"

	"audiobookshelf/internal/core"
)

// handleGetAuthSettings maps to GET /api/auth-settings
func handleGetAuthSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/auth-settings")
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
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

		authSettings := buildAuthSettingsResponse(currentSettings)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(authSettings)
	}
}

// handleUpdateAuthSettings maps to PATCH /api/auth-settings
func handleUpdateAuthSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] PATCH /api/auth-settings")
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

		allowedKeys := []string{
			"authLoginCustomMessage", "authActiveAuthMethods", "authOpenIDIssuerURL", "authOpenIDAuthorizationURL",
			"authOpenIDTokenURL", "authOpenIDUserInfoURL", "authOpenIDJwksURL", "authOpenIDLogoutURL",
			"authOpenIDClientID", "authOpenIDClientSecret", "authOpenIDTokenSigningAlgorithm",
			"authOpenIDButtonText", "authOpenIDAutoLaunch", "authOpenIDAutoRegister", "authOpenIDMatchExistingBy",
			"authOpenIDMobileRedirectURIs", "authOpenIDGroupClaim", "authOpenIDAdvancedPermsClaim", "authOpenIDSubfolderForRedirectURLs",
		}

		hasUpdates := false
		for _, key := range allowedKeys {
			if val, ok := settingsUpdate[key]; ok {
				currentSettings[key] = val
				hasUpdates = true
			}
		}

		if hasUpdates {
			if err := saveSettings(db, "server-settings", currentSettings); err != nil {
				http.Error(w, `{"error": "Failed to update auth settings"}`, http.StatusInternalServerError)
				return
			}
		}

		browserSettings := sanitizeBrowserSettings(currentSettings)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"updated":        hasUpdates,
			"serverSettings": browserSettings,
		})
	}
}

// buildAuthSettingsResponse extracts auth settings fields and populates defaults
func buildAuthSettingsResponse(settings map[string]interface{}) map[string]interface{} {
	authSettings := map[string]interface{}{
		"authLoginCustomMessage":             settings["authLoginCustomMessage"],
		"authActiveAuthMethods":              settings["authActiveAuthMethods"],
		"authOpenIDIssuerURL":                settings["authOpenIDIssuerURL"],
		"authOpenIDAuthorizationURL":         settings["authOpenIDAuthorizationURL"],
		"authOpenIDTokenURL":                 settings["authOpenIDTokenURL"],
		"authOpenIDUserInfoURL":              settings["authOpenIDUserInfoURL"],
		"authOpenIDJwksURL":                  settings["authOpenIDJwksURL"],
		"authOpenIDLogoutURL":                settings["authOpenIDLogoutURL"],
		"authOpenIDClientID":                 settings["authOpenIDClientID"],
		"authOpenIDClientSecret":             settings["authOpenIDClientSecret"],
		"authOpenIDTokenSigningAlgorithm":    settings["authOpenIDTokenSigningAlgorithm"],
		"authOpenIDButtonText":               settings["authOpenIDButtonText"],
		"authOpenIDAutoLaunch":               settings["authOpenIDAutoLaunch"],
		"authOpenIDAutoRegister":             settings["authOpenIDAutoRegister"],
		"authOpenIDMatchExistingBy":          settings["authOpenIDMatchExistingBy"],
		"authOpenIDMobileRedirectURIs":       settings["authOpenIDMobileRedirectURIs"],
		"authOpenIDGroupClaim":               settings["authOpenIDGroupClaim"],
		"authOpenIDAdvancedPermsClaim":       settings["authOpenIDAdvancedPermsClaim"],
		"authOpenIDSubfolderForRedirectURLs": settings["authOpenIDSubfolderForRedirectURLs"],
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

	return authSettings
}
