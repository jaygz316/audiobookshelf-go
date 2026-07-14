package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"audiobookshelf/internal/core"
	log "audiobookshelf/internal/logger"
)

// User represents the full user structure matching the SQLite table
type User struct {
	ID          string          `json:"id"`
	Username    string          `json:"username"`
	Email       *string         `json:"email"`
	Pash        string          `json:"-"`
	Type        string          `json:"type"`
	Token       string          `json:"token"`
	IsActive    bool            `json:"isActive"`
	IsLocked    bool            `json:"isLocked"`
	LastSeen    *int64          `json:"lastSeen"`
	Permissions json.RawMessage `json:"permissions"`
	Bookmarks   json.RawMessage `json:"bookmarks"`
	ExtraData   json.RawMessage `json:"extraData"`
	CreatedAt   int64           `json:"createdAt"`
	UpdatedAt   int64           `json:"updatedAt"`
}

// UserPermissionsDetailed corresponds to the parsed permissions object stored in the DB
type UserPermissionsDetailed struct {
	Download                  *bool    `json:"download,omitempty"`
	AccessExplicitContent     *bool    `json:"accessExplicitContent,omitempty"`
	AccessAllLibraries        *bool    `json:"accessAllLibraries,omitempty"`
	LibrariesAccessible       []string `json:"librariesAccessible,omitempty"`
	AccessAllTags             *bool    `json:"accessAllTags,omitempty"`
	ItemTagsSelected          []string `json:"itemTagsSelected,omitempty"`
	SelectedTagsNotAccessible *bool    `json:"selectedTagsNotAccessible,omitempty"`
}

// UserSessionDB matches the sessions table
type UserSessionDB struct {
	ID                        string `db:"id"`
	IPAddress                 string `db:"ipAddress"`
	UserAgent                 string `db:"userAgent"`
	RefreshToken              string `db:"refreshToken"`
	ExpiresAt                 int64  `db:"expiresAt"`
	LastRefreshToken          string `db:"lastRefreshToken"`
	LastRefreshTokenExpiresAt int64  `db:"lastRefreshTokenExpiresAt"`
	UserID                    string `db:"userId"`
	CreatedAt                 int64  `db:"createdAt"`
	UpdatedAt                 int64  `db:"updatedAt"`
}

// ToOldJSONForBrowser maps User to the format client expects
func (u *User) ToOldJSONForBrowser(hideRootToken bool) map[string]interface{} {
	var perms map[string]interface{}
	if len(u.Permissions) > 0 {
		json.Unmarshal(u.Permissions, &perms)
	}
	if perms == nil {
		perms = make(map[string]interface{})
	}

	librariesAccessible := []string{}
	if libs, ok := perms["librariesAccessible"]; ok {
		if libsArr, ok2 := libs.([]interface{}); ok2 {
			for _, libVal := range libsArr {
				if libStr, ok3 := libVal.(string); ok3 {
					librariesAccessible = append(librariesAccessible, libStr)
				}
			}
		}
	}
	itemTagsSelected := []string{}
	if tags, ok := perms["itemTagsSelected"]; ok {
		if tagsArr, ok2 := tags.([]interface{}); ok2 {
			for _, tagVal := range tagsArr {
				if tagStr, ok3 := tagVal.(string); ok3 {
					itemTagsSelected = append(itemTagsSelected, tagStr)
				}
			}
		}
	}

	delete(perms, "librariesAccessible")
	delete(perms, "itemTagsSelected")

	var extra map[string]interface{}
	if len(u.ExtraData) > 0 {
		json.Unmarshal(u.ExtraData, &extra)
	}
	if extra == nil {
		extra = make(map[string]interface{})
	}

	seriesHideFromContinueListening := []string{}
	if hfc, ok := extra["seriesHideFromContinueListening"]; ok {
		if hfcArr, ok2 := hfc.([]interface{}); ok2 {
			for _, hVal := range hfcArr {
				if hStr, ok3 := hVal.(string); ok3 {
					seriesHideFromContinueListening = append(seriesHideFromContinueListening, hStr)
				}
			}
		}
	}

	var bookmarksArr []interface{}
	if len(u.Bookmarks) > 0 {
		json.Unmarshal(u.Bookmarks, &bookmarksArr)
	}
	if bookmarksArr == nil {
		bookmarksArr = []interface{}{}
	}

	token := u.Token
	if u.Type == "root" && hideRootToken {
		token = ""
	}

	hasOpenIDLink := false
	if oSub, ok := extra["authOpenIDSub"]; ok && oSub != nil && oSub != "" {
		hasOpenIDLink = true
	}

	return map[string]interface{}{
		"id":                              u.ID,
		"username":                        u.Username,
		"email":                           u.Email,
		"type":                            u.Type,
		"token":                           token,
		"isOldToken":                      false,
		"mediaProgress":                   []interface{}{}, // Loaded separately if requested
		"seriesHideFromContinueListening": seriesHideFromContinueListening,
		"bookmarks":                       bookmarksArr,
		"isActive":                        u.IsActive,
		"isLocked":                        u.IsLocked,
		"lastSeen":                        u.LastSeen,
		"createdAt":                       u.CreatedAt,
		"permissions":                     perms,
		"librariesAccessible":             librariesAccessible,
		"itemTagsSelected":                itemTagsSelected,
		"hasOpenIDLink":                   hasOpenIDLink,
	}
}

// Database Helpers for Users

func GetUserFullByUsername(ctx context.Context, db *sql.DB, username string) (*User, error) {
	row := db.QueryRowContext(ctx, "SELECT id, username, email, pash, type, token, isActive, isLocked, lastSeen, permissions, bookmarks, extraData, createdAt, updatedAt FROM users WHERE username = ?", username)
	return ScanUser(row)
}

func GetUserFullByID(ctx context.Context, db *sql.DB, id string) (*User, error) {
	row := db.QueryRowContext(ctx, "SELECT id, username, email, pash, type, token, isActive, isLocked, lastSeen, permissions, bookmarks, extraData, createdAt, updatedAt FROM users WHERE id = ?", id)
	return ScanUser(row)
}

func CheckUserExistsWithUsername(ctx context.Context, db *sql.DB, username string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE username = ?", username).Scan(&count)
	return count > 0, err
}

func ScanUser(row *sql.Row) (*User, error) {
	var u User
	var email sql.NullString
	var lastSeenStr sql.NullString
	var isActiveInt, isLockedInt sql.NullInt64
	var permsStr, bookmarksStr, extraDataStr sql.NullString
	var createdAtStr, updatedAtStr sql.NullString
	var pashStr, tokenStr, typeStr sql.NullString

	err := row.Scan(&u.ID, &u.Username, &email, &pashStr, &typeStr, &tokenStr, &isActiveInt, &isLockedInt, &lastSeenStr, &permsStr, &bookmarksStr, &extraDataStr, &createdAtStr, &updatedAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
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
	} else {
		u.Permissions = []byte("{}")
	}

	if bookmarksStr.Valid {
		u.Bookmarks = []byte(bookmarksStr.String)
	} else {
		u.Bookmarks = []byte("[]")
	}

	if extraDataStr.Valid {
		u.ExtraData = []byte(extraDataStr.String)
	} else {
		u.ExtraData = []byte("{}")
	}

	u.CreatedAt = ParseTimeStr(createdAtStr.String)
	u.UpdatedAt = ParseTimeStr(updatedAtStr.String)

	return &u, nil
}

func ParseTimeStr(s string) int64 {
	if s == "" {
		return 0
	}
	// Try parsing as raw integer first (milliseconds timestamp)
	if val, err := strconv.ParseInt(s, 10, 64); err == nil {
		return val
	}
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t.UnixNano() / int64(time.Millisecond)
	}
	t2, err2 := time.Parse("2006-01-02 15:04:05.000 +00:00", s)
	if err2 == nil {
		return t2.UnixNano() / int64(time.Millisecond)
	}
	t3, err3 := time.Parse("2006-01-02 15:04:05", s)
	if err3 == nil {
		return t3.UnixNano() / int64(time.Millisecond)
	}
	return 0
}

func TimeToDBStr(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05.000 +00:00")
}

// GetDefaultPermissionsForUserType maps type to default permissions JSON
func GetDefaultPermissionsForUserType(userType string) string {
	isAccess := false
	if userType == "root" || userType == "admin" {
		isAccess = true
	}
	perms := map[string]interface{}{
		"download":                  true,
		"accessExplicitContent":     isAccess,
		"accessAllLibraries":        true,
		"librariesAccessible":       []string{},
		"accessAllTags":             true,
		"itemTagsSelected":          []string{},
		"selectedTagsNotAccessible": false,
	}
	bytes, _ := json.Marshal(perms)
	return string(bytes)
}

// Database Helpers for Sessions

func CreateSession(ctx context.Context, db *sql.DB, userID, ipAddress, userAgent, refreshToken string, expiresAt time.Time) error {
	sessionID := uuid.New().String()
	nowStr := TimeToDBStr(time.Now())
	expiresStr := TimeToDBStr(expiresAt)
	_, err := db.ExecContext(ctx, `INSERT INTO sessions (id, userId, ipAddress, userAgent, refreshToken, expiresAt, lastRefreshToken, lastRefreshTokenExpiresAt, createdAt, updatedAt) 
		VALUES (?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?)`,
		sessionID, userID, ipAddress, userAgent, refreshToken, expiresStr, nowStr, nowStr)
	return err
}

func DeleteSessionByRefreshToken(ctx context.Context, db *sql.DB, refreshToken string) (int64, error) {
	res, err := db.ExecContext(ctx, "DELETE FROM sessions WHERE refreshToken = ?", refreshToken)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func CleanupExpiredSessions(ctx context.Context, db *sql.DB) (int64, error) {
	nowStr := TimeToDBStr(time.Now())
	res, err := db.ExecContext(ctx, "DELETE FROM sessions WHERE expiresAt < ?", nowStr)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

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

func FindUserFromOpenIdUserInfo(ctx context.Context, db *sql.DB, userinfo map[string]interface{}, matchBy string) (*User, error) {
	sub, _ := userinfo["sub"].(string)
	if sub == "" {
		return nil, fmt.Errorf("invalid userinfo, no sub")
	}

	var u *User
	row := db.QueryRowContext(ctx, "SELECT id, username, email, pash, type, token, isActive, isLocked, lastSeen, permissions, bookmarks, extraData, createdAt, updatedAt FROM users WHERE json_extract(extraData, '$.authOpenIDSub') = ?", sub)
	u, err := ScanUser(row)
	if err == nil && u != nil {
		log.Printf("[User] openid: User found by sub %s", sub)
		return u, nil
	}

	if matchBy == "email" {
		email, _ := userinfo["email"].(string)
		if email != "" {
			if verified, ok := userinfo["email_verified"].(bool); ok && !verified {
				return nil, fmt.Errorf("email not verified")
			}
			row = db.QueryRowContext(ctx, "SELECT id, username, email, pash, type, token, isActive, isLocked, lastSeen, permissions, bookmarks, extraData, createdAt, updatedAt FROM users WHERE lower(email) = ?", strings.ToLower(email))
			u, err = ScanUser(row)
			if err == nil && u != nil {
				var extra map[string]interface{}
				json.Unmarshal(u.ExtraData, &extra)
				if oSub, ok := extra["authOpenIDSub"]; ok && oSub != nil && oSub != "" && oSub != sub {
					return nil, fmt.Errorf("user already linked to a different OpenID subject")
				}
				if extra == nil {
					extra = make(map[string]interface{})
				}
				extra["authOpenIDSub"] = sub
				extraBytes, _ := json.Marshal(extra)
				_, err = db.ExecContext(ctx, "UPDATE users SET extraData = ? WHERE id = ?", string(extraBytes), u.ID)
				if err != nil {
					return nil, err
				}
				u.ExtraData = extraBytes
				return u, nil
			}
		} else {
			return nil, fmt.Errorf("no email in userinfo")
		}
	} else if matchBy == "username" {
		var username string
		if pu, ok := userinfo["preferred_username"].(string); ok && pu != "" {
			username = pu
		} else if un, ok := userinfo["username"].(string); ok && un != "" {
			username = un
		} else if name, ok := userinfo["name"].(string); ok && name != "" {
			username = name
		}
		if username != "" {
			row = db.QueryRowContext(ctx, "SELECT id, username, email, pash, type, token, isActive, isLocked, lastSeen, permissions, bookmarks, extraData, createdAt, updatedAt FROM users WHERE lower(username) = ?", strings.ToLower(username))
			u, err = ScanUser(row)
			if err == nil && u != nil {
				var extra map[string]interface{}
				json.Unmarshal(u.ExtraData, &extra)
				if oSub, ok := extra["authOpenIDSub"]; ok && oSub != nil && oSub != "" && oSub != sub {
					return nil, fmt.Errorf("user already linked to a different OpenID subject")
				}
				if extra == nil {
					extra = make(map[string]interface{})
				}
				extra["authOpenIDSub"] = sub
				extraBytes, _ := json.Marshal(extra)
				_, err = db.ExecContext(ctx, "UPDATE users SET extraData = ? WHERE id = ?", string(extraBytes), u.ID)
				if err != nil {
					return nil, err
				}
				u.ExtraData = extraBytes
				return u, nil
			}
		} else {
			return nil, fmt.Errorf("no username in userinfo")
		}
	}

	return nil, nil
}

func CreateUserFromOpenIdUserInfo(ctx context.Context, db *sql.DB, userinfo map[string]interface{}, tokenSecret string, userType string) (*User, error) {
	userId := uuid.New().String()
	username, _ := userinfo["preferred_username"].(string)
	if username == "" {
		username, _ = userinfo["name"].(string)
	}
	if username == "" {
		username, _ = userinfo["sub"].(string)
	}

	var emailVal *string
	if email, ok := userinfo["email"].(string); ok && email != "" {
		if ev, ok2 := userinfo["email_verified"].(bool); !ok2 || ev {
			emailVal = &email
		}
	}

	claims := &core.AuthClaims{
		UserID:   userId,
		Username: username,
		Type:     userType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "audiobookshelf",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(tokenSecret))
	if err != nil {
		return nil, err
	}

	extra := map[string]interface{}{
		"authOpenIDSub":                   userinfo["sub"],
		"seriesHideFromContinueListening": []string{},
	}
	extraBytes, _ := json.Marshal(extra)
	if userType == "" {
		userType = "user"
	}
	perms := GetDefaultPermissionsForUserType(userType)
	nowStr := TimeToDBStr(time.Now())

	var emailStr sql.NullString
	if emailVal != nil {
		emailStr.String = *emailVal
		emailStr.Valid = true
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO users (id, username, email, pash, type, token, isActive, isLocked, permissions, bookmarks, extraData, createdAt, updatedAt)
		VALUES (?, ?, ?, NULL, ?, ?, 1, 0, ?, '[]', ?, ?, ?)`,
		userId, username, emailStr, userType, tokenString, perms, string(extraBytes), nowStr, nowStr)
	if err != nil {
		return nil, err
	}

	return &User{
		ID:          userId,
		Username:    username,
		Email:       emailVal,
		Type:        userType,
		Token:       tokenString,
		IsActive:    true,
		IsLocked:    false,
		Permissions: []byte(perms),
		Bookmarks:   []byte("[]"),
		ExtraData:   extraBytes,
		CreatedAt:   time.Now().UnixNano() / int64(time.Millisecond),
		UpdatedAt:   time.Now().UnixNano() / int64(time.Millisecond),
	}, nil
}

func UpdateUserTypeAndToken(ctx context.Context, db *sql.DB, u *User, newType string, tokenSecret string) error {
	claims := &core.AuthClaims{
		UserID:   u.ID,
		Username: u.Username,
		Type:     newType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "audiobookshelf",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(tokenSecret))
	if err != nil {
		return err
	}

	perms := GetDefaultPermissionsForUserType(newType)
	_, err = db.ExecContext(ctx, "UPDATE users SET type = ?, token = ?, permissions = ?, updatedAt = ? WHERE id = ?",
		newType, tokenString, perms, TimeToDBStr(time.Now()), u.ID)
	if err != nil {
		return err
	}

	u.Type = newType
	u.Token = tokenString
	u.Permissions = []byte(perms)
	return nil
}

// GetUserSessions retrieves all active login sessions for the given user ID.
func GetUserSessions(ctx context.Context, db *sql.DB, userID string) ([]UserSessionDB, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, userId, ipAddress, userAgent, refreshToken, expiresAt, lastRefreshToken, lastRefreshTokenExpiresAt, createdAt, updatedAt FROM sessions WHERE userId = ? ORDER BY COALESCE(updatedAt, createdAt) DESC", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []UserSessionDB
	for rows.Next() {
		var s UserSessionDB
		var expiresAtStr string
		var lastExpiresAtStr sql.NullString
		var lastRefreshToken sql.NullString
		var createdAtStr, updatedAtStr sql.NullString

		err := rows.Scan(&s.ID, &s.UserID, &s.IPAddress, &s.UserAgent, &s.RefreshToken, &expiresAtStr, &lastRefreshToken, &lastExpiresAtStr, &createdAtStr, &updatedAtStr)
		if err != nil {
			return nil, err
		}

		s.ExpiresAt = ParseTimeStr(expiresAtStr)
		if lastRefreshToken.Valid {
			s.LastRefreshToken = lastRefreshToken.String
		}
		if lastExpiresAtStr.Valid {
			s.LastRefreshTokenExpiresAt = ParseTimeStr(lastExpiresAtStr.String)
		}
		if createdAtStr.Valid {
			s.CreatedAt = ParseTimeStr(createdAtStr.String)
		}
		if updatedAtStr.Valid {
			s.UpdatedAt = ParseTimeStr(updatedAtStr.String)
		}

		sessions = append(sessions, s)
	}

	if sessions == nil {
		sessions = []UserSessionDB{}
	}
	return sessions, nil
}

// DeleteSessionByID deletes a login session from the sessions table by ID.
func DeleteSessionByID(ctx context.Context, db *sql.DB, sessionID string) error {
	_, err := db.ExecContext(ctx, "DELETE FROM sessions WHERE id = ?", sessionID)
	return err
}
