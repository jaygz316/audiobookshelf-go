package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"audiobookshelf/internal/core"
)

// ServerSettings holds the settings stored in the database.
type ServerSettings struct {
	TokenSecret            string   `json:"tokenSecret"`
	Language               string   `json:"language"`
	AuthActiveAuthMethods  []string `json:"authActiveAuthMethods"`
	AuthLoginCustomMessage *string  `json:"authLoginCustomMessage"`
	BackupPath             string   `json:"backupPath"`
	BackupsToKeep          int      `json:"backupsToKeep"`
}

// GetServerSettings reads the server settings from the settings table.
func GetServerSettings(database *sql.DB) (*ServerSettings, error) {
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var valStr string
	err := database.QueryRow("SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
	if err != nil {
		return nil, err
	}

	var settings ServerSettings
	if err := json.Unmarshal([]byte(valStr), &settings); err != nil {
		return nil, err
	}

	// Fallback to defaults
	if len(settings.AuthActiveAuthMethods) == 0 {
		settings.AuthActiveAuthMethods = []string{"local"}
	}
	if settings.Language == "" {
		settings.Language = "en-us"
	}

	return &settings, nil
}

// GetSortingIgnorePrefix reads sortingIgnorePrefix from server-settings.
func GetSortingIgnorePrefix(database *sql.DB) bool {
	if database == nil {
		return false
	}
	var valStr string
	err := database.QueryRow("SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
	if err != nil {
		return false
	}
	var s struct {
		SortingIgnorePrefix bool `json:"sortingIgnorePrefix"`
	}
	if err := json.Unmarshal([]byte(valStr), &s); err != nil {
		return false
	}
	return s.SortingIgnorePrefix
}

// HasRootUser checks if any user of type 'root' exists in the users table.
func HasRootUser(database *sql.DB) (bool, error) {
	if database == nil {
		return false, fmt.Errorf("database not initialized")
	}
	var count int
	err := database.QueryRow("SELECT count(*) FROM users WHERE type = 'root'").Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ParseSQLiteTime parses a SQLite timestamp string into a time.Time.
func ParseSQLiteTime(s string) (time.Time, error) {
	layouts := []string{
		"2006-01-02 15:04:05.000 +00:00",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02 15:04:05.000000 +00:00",
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("failed to parse time: %s", s)
}

// ParseEpochMillis parses a SQLite timestamp string into Unix epoch milliseconds.
func ParseEpochMillis(s string) int64 {
	t, err := ParseSQLiteTime(s)
	if err != nil {
		return 0
	}
	return t.UnixNano() / int64(time.Millisecond)
}

// userPermissions is the internal struct for parsing DB permissions JSON.
type userPermissions struct {
	Download                  *bool    `json:"download"`
	AccessExplicitContent     *bool    `json:"accessExplicitContent"`
	AccessAllLibraries        *bool    `json:"accessAllLibraries"`
	LibrariesAccessible       []string `json:"librariesAccessible"`
	Libraries                 []string `json:"libraries"`
	AccessAllTags             *bool    `json:"accessAllTags"`
	ItemTagsSelected          []string `json:"itemTagsSelected"`
	SelectedTagsNotAccessible *bool    `json:"selectedTagsNotAccessible"`
}

// ParsePermissions parses the permissions JSON string into a UserSession.
func ParsePermissions(permsStr sql.NullString, user *core.UserSession) {
	// default values:
	user.CanDownload = true
	user.CanAccessExplicitContent = false
	user.AccessAllLibraries = true
	user.LibrariesAccessible = []string{}
	user.AccessAllTags = true
	user.ItemTagsSelected = []string{}
	user.SelectedTagsNotAccessible = false

	// if it's admin or root, they have all access by default
	if user.Type == "root" || user.Type == "admin" {
		user.CanAccessExplicitContent = true
		user.AccessAllLibraries = true
		user.AccessAllTags = true
	}

	if !permsStr.Valid || permsStr.String == "" {
		return
	}

	var perms userPermissions
	if err := json.Unmarshal([]byte(permsStr.String), &perms); err != nil {
		return
	}

	if perms.Download != nil {
		user.CanDownload = *perms.Download
	}
	if perms.AccessExplicitContent != nil {
		user.CanAccessExplicitContent = *perms.AccessExplicitContent
	}
	if perms.AccessAllLibraries != nil {
		user.AccessAllLibraries = *perms.AccessAllLibraries
	}
	if perms.LibrariesAccessible != nil {
		user.LibrariesAccessible = perms.LibrariesAccessible
		if perms.AccessAllLibraries == nil {
			user.AccessAllLibraries = false
		}
	} else if perms.Libraries != nil {
		user.LibrariesAccessible = perms.Libraries
		if perms.AccessAllLibraries == nil {
			user.AccessAllLibraries = false
		}
	}
	if perms.AccessAllTags != nil {
		user.AccessAllTags = *perms.AccessAllTags
	}
	if perms.ItemTagsSelected != nil {
		user.ItemTagsSelected = perms.ItemTagsSelected
	}
	if perms.SelectedTagsNotAccessible != nil {
		user.SelectedTagsNotAccessible = *perms.SelectedTagsNotAccessible
	}
}

// GetUserByID fetches minimum info needed for authentication for a user ID.
func GetUserByID(database *sql.DB, userID string) (*core.UserSession, error) {
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var user core.UserSession
	var isActiveInt int
	var permsStr sql.NullString
	err := database.QueryRow("SELECT id, username, type, isActive, permissions FROM users WHERE id = ?", userID).
		Scan(&user.ID, &user.Username, &user.Type, &isActiveInt, &permsStr)
	if err != nil {
		return nil, err
	}
	user.IsActive = isActiveInt != 0
	ParsePermissions(permsStr, &user)
	return &user, nil
}

// GetUserByIDOrOldID fetches minimum info needed for authentication for a user ID or old user ID.
func GetUserByIDOrOldID(database *sql.DB, userID string) (*core.UserSession, error) {
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var user core.UserSession
	var isActiveInt int
	var extraDataStr string
	var permsStr sql.NullString

	err := database.QueryRow("SELECT id, username, type, isActive, extraData, permissions FROM users WHERE id = ?", userID).
		Scan(&user.ID, &user.Username, &user.Type, &isActiveInt, &extraDataStr, &permsStr)

	if err == sql.ErrNoRows {
		err = database.QueryRow("SELECT id, username, type, isActive, extraData, permissions FROM users WHERE json_extract(extraData, '$.oldUserId') = ?", userID).
			Scan(&user.ID, &user.Username, &user.Type, &isActiveInt, &extraDataStr, &permsStr)
	}

	if err != nil {
		return nil, err
	}
	user.IsActive = isActiveInt != 0
	ParsePermissions(permsStr, &user)
	return &user, nil
}

// CheckAPIKey verifies that an API key is active and not expired.
func CheckAPIKey(database *sql.DB, keyID string) (*core.UserSession, error) {
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var isActiveInt int
	var expiresAtStr sql.NullString
	var userID string

	err := database.QueryRow("SELECT isActive, expiresAt, userId FROM apiKeys WHERE id = ?", keyID).
		Scan(&isActiveInt, &expiresAtStr, &userID)
	if err != nil {
		return nil, err
	}

	if isActiveInt == 0 {
		return nil, fmt.Errorf("API key is inactive")
	}

	if expiresAtStr.Valid && expiresAtStr.String != "" {
		expiresAt, parseErr := ParseSQLiteTime(expiresAtStr.String)
		if parseErr != nil {
			return nil, fmt.Errorf("failed to parse API key expiry: %v", parseErr)
		}
		if time.Now().After(expiresAt) {
			return nil, fmt.Errorf("API key has expired")
		}
	}

	return GetUserByID(database, userID)
}

// JsonArrayToCommaString converts a JSON array of strings to a comma-separated string.
func JsonArrayToCommaString(jsonBytes []byte) string {
	if len(jsonBytes) == 0 {
		return ""
	}
	var arr []string
	if err := json.Unmarshal(jsonBytes, &arr); err != nil {
		return ""
	}
	result := ""
	for i, s := range arr {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}

// GetTokenSecret retrieves the JWT token secret from the environment or database settings.
func GetTokenSecret(database *sql.DB) string {
	if envSecret := os.Getenv("JWT_SECRET_KEY"); envSecret != "" {
		return envSecret
	}
	if database == nil {
		return ""
	}
	settings, err := GetServerSettings(database)
	if err == nil && settings != nil && settings.TokenSecret != "" {
		return settings.TokenSecret
	}
	return ""
}
