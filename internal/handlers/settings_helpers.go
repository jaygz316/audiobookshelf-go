package handlers

import (
	idb "audiobookshelf/internal/db"
	"database/sql"
	"encoding/json"
	"time"
)

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
	if browserSettings["theme"] == nil || browserSettings["theme"] == "" {
		browserSettings["theme"] = "dark"
	}
	if browserSettings["customCss"] == nil {
		browserSettings["customCss"] = ""
	}
	return browserSettings
}
