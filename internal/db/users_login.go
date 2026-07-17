package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"

	"audiobookshelf/internal/core"
	log "audiobookshelf/internal/logger"
)

func GetUserLoginPayload(ctx context.Context, db *sql.DB, user *User) (map[string]interface{}, error) {
	// 1. Get serverSettings
	var valStr string
	err := db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
	currentSettings := make(map[string]interface{})
	if err == nil && valStr != "" {
		json.Unmarshal([]byte(valStr), &currentSettings)
	}

	browserSettings := make(map[string]interface{})
	for k, v := range currentSettings {
		if k != "tokenSecret" && k != "authOpenIDClientID" && k != "authOpenIDClientSecret" &&
			k != "authOpenIDMobileRedirectURIs" && k != "authOpenIDGroupClaim" && k != "authOpenIDAdvancedPermsClaim" {
			browserSettings[k] = v
		}
	}
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

	// 2. Get libraries filtered by user's access
	userSess := &core.UserSession{
		ID:   user.ID,
		Type: user.Type,
	}
	ParsePermissions(sql.NullString{String: string(user.Permissions), Valid: len(user.Permissions) > 0}, userSess)

	libs, err := GetLibraries(db)
	var filteredLibs []*LibraryJSON
	if err == nil {
		for _, lib := range libs {
			if userSess.CanAccessLibrary(lib.ID) {
				filteredLibs = append(filteredLibs, lib)
			}
		}
	} else {
		filteredLibs = []*LibraryJSON{}
	}

	var defaultLibraryID string
	if len(filteredLibs) > 0 {
		defaultLibraryID = filteredLibs[0].ID
	}

	source := os.Getenv("SOURCE")
	if source == "" {
		source = "debian"
	}

	payload := map[string]interface{}{
		"serverSettings":       browserSettings,
		"libraries":            filteredLibs,
		"userDefaultLibraryId": defaultLibraryID,
		"Source":               source,
		"ereaderDevices":       []interface{}{},
	}

	// 3. If root/admin, return all users
	if user.Type == "root" || user.Type == "admin" {
		hideRootToken := user.Type != "root"
		rows, err := db.QueryContext(ctx, "SELECT id, username, email, pash, type, token, isActive, isLocked, lastSeen, permissions, bookmarks, extraData, createdAt, updatedAt FROM users ORDER BY username ASC")
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var usersJSON []map[string]interface{}
		for rows.Next() {
			var u User
			var email sql.NullString
			var lastSeenStr sql.NullString
			var isActiveInt, isLockedInt sql.NullInt64
			var permsStr, bookmarksStr, extraDataStr sql.NullString
			var createdAtStr, updatedAtStr sql.NullString
			var pashStr, tokenStr, typeStr sql.NullString

			err := rows.Scan(&u.ID, &u.Username, &email, &pashStr, &typeStr, &tokenStr, &isActiveInt, &isLockedInt, &lastSeenStr, &permsStr, &bookmarksStr, &extraDataStr, &createdAtStr, &updatedAtStr)
			if err != nil {
				log.Printf("[Users] Failed to scan user for login payload: %v", err)
				continue
			}

			if typeStr.Valid {
				u.Type = typeStr.String
			} else {
				u.Type = "user"
			}

			if pashStr.Valid {
				u.Pash = pashStr.String
			}
			if tokenStr.Valid {
				u.Token = tokenStr.String
			}
			if email.Valid {
				u.Email = &email.String
			}
			u.IsActive = isActiveInt.Valid && isActiveInt.Int64 != 0
			u.IsLocked = isLockedInt.Valid && isLockedInt.Int64 != 0
			if lastSeenStr.Valid && lastSeenStr.String != "" {
				val := ParseTimeStr(lastSeenStr.String)
				u.LastSeen = &val
			}
			if permsStr.Valid {
				u.Permissions = []byte(permsStr.String)
			}
			if bookmarksStr.Valid {
				u.Bookmarks = []byte(bookmarksStr.String)
			}
			if extraDataStr.Valid {
				u.ExtraData = []byte(extraDataStr.String)
			}
			u.CreatedAt = ParseTimeStr(createdAtStr.String)
			u.UpdatedAt = ParseTimeStr(updatedAtStr.String)

			usersJSON = append(usersJSON, u.ToOldJSONForBrowser(hideRootToken))
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		payload["users"] = usersJSON
	}

	return payload, nil
}
