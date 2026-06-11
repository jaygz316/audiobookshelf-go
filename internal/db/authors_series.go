package db

import (
	"database/sql"
	"encoding/json"
)

// GetAuthorImagePath retrieves the imagePath for a specific author
func GetAuthorImagePath(db *sql.DB, authorID string) (string, error) {
	var imagePath sql.NullString
	err := db.QueryRow("SELECT imagePath FROM authors WHERE id = ?", authorID).Scan(&imagePath)
	if err != nil {
		return "", err
	}
	if !imagePath.Valid {
		return "", nil
	}
	return imagePath.String, nil
}

// GetSortingPrefixes retrieves the sorting prefixes from server-settings.
func GetSortingPrefixes(db *sql.DB) []string {
	var valStr string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
	if err != nil {
		return []string{"the", "a"}
	}
	var settings struct {
		SortingPrefixes []string `json:"sortingPrefixes"`
	}
	if err := json.Unmarshal([]byte(valStr), &settings); err == nil {
		return settings.SortingPrefixes
	}
	return []string{"the", "a"}
}
