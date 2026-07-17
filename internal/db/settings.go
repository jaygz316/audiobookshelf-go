package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// BackupScheduleType is a custom type that can unmarshal from either a boolean or a string.
type BackupScheduleType string

func (bst *BackupScheduleType) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		*bst = ""
		return nil
	}
	if string(data) == "false" {
		*bst = ""
		return nil
	}
	if string(data) == "true" {
		*bst = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*bst = BackupScheduleType(s)
	return nil
}

func (bst BackupScheduleType) MarshalJSON() ([]byte, error) {
	if bst == "" {
		return []byte("false"), nil
	}
	return json.Marshal(string(bst))
}

// ServerSettings holds the settings stored in the database.
type ServerSettings struct {
	TokenSecret                  string             `json:"tokenSecret"`
	Language                     string             `json:"language"`
	AuthActiveAuthMethods        []string           `json:"authActiveAuthMethods"`
	AuthLoginCustomMessage       *string            `json:"authLoginCustomMessage"`
	BackupPath                   string             `json:"backupPath"`
	BackupsToKeep                int                `json:"backupsToKeep"`
	BackupSchedule               BackupScheduleType `json:"backupSchedule"`
	MetadataCoverWithItem        bool               `json:"metadataCoverWithItem"`
	MetadataMarkdownWithItem     bool               `json:"metadataMarkdownWithItem"`
	SortingIgnorePrefix          bool               `json:"sortingIgnorePrefix"`
	ScannerParseSubtitles        bool               `json:"scannerParseSubtitles"`
	ScannerFindCovers            bool               `json:"scannerFindCovers"`
	ScannerCoverProvider         string             `json:"scannerCoverProvider"`
	ScannerPreferMatchedMetadata bool               `json:"scannerPreferMatchedMetadata"`
	WatchLibraryChanges          bool               `json:"watchLibraryChanges"`
	ChromecastEnabled            bool               `json:"chromecastEnabled"`
	AllowIframe                  bool               `json:"allowIframe"`
	HomePageBookshelfView        bool               `json:"homePageBookshelfView"`
	LibraryBookshelfView         bool               `json:"libraryBookshelfView"`
	DateFormat                   string             `json:"dateFormat"`
	TimeFormat                   string             `json:"timeFormat"`
	AllowedCorsOrigins           string             `json:"allowedCorsOrigins"`
	Theme                        string             `json:"theme"`
	CustomCSS                    string             `json:"customCss"`
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
	settings.SortingIgnorePrefix = true
	settings.ScannerParseSubtitles = true
	settings.ScannerFindCovers = true
	settings.WatchLibraryChanges = true
	settings.DateFormat = "MM/DD/YYYY"
	settings.TimeFormat = "HH:mm"
	settings.Theme = "dark"

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
	if settings.Theme == "" {
		settings.Theme = "dark"
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
