package db

import (
	"database/sql"
	"fmt"
	"time"

	"audiobookshelf/internal/core"
)

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
	var extraDataStr sql.NullString
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
