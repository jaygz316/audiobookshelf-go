package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
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

// toOldJSONForBrowser maps User to the format client expects
func (u *User) toOldJSONForBrowser(hideRootToken bool) map[string]interface{} {
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

func getUserByUsername(ctx context.Context, db *sql.DB, username string) (*User, error) {
	row := db.QueryRowContext(ctx, "SELECT id, username, email, pash, type, token, isActive, isLocked, lastSeen, permissions, bookmarks, extraData, createdAt, updatedAt FROM users WHERE username = ?", username)
	return scanUser(row)
}

func getUserByID(ctx context.Context, db *sql.DB, id string) (*User, error) {
	row := db.QueryRowContext(ctx, "SELECT id, username, email, pash, type, token, isActive, isLocked, lastSeen, permissions, bookmarks, extraData, createdAt, updatedAt FROM users WHERE id = ?", id)
	return scanUser(row)
}

func checkUserExistsWithUsername(ctx context.Context, db *sql.DB, username string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE username = ?", username).Scan(&count)
	return count > 0, err
}

func scanUser(row *sql.Row) (*User, error) {
	var u User
	var email sql.NullString
	var lastSeenStr sql.NullString
	var isActiveInt, isLockedInt sql.NullInt64
	var permsStr, bookmarksStr, extraDataStr sql.NullString
	var createdAtStr, updatedAtStr sql.NullString
	var pashStr, tokenStr sql.NullString

	err := row.Scan(&u.ID, &u.Username, &email, &pashStr, &u.Type, &tokenStr, &isActiveInt, &isLockedInt, &lastSeenStr, &permsStr, &bookmarksStr, &extraDataStr, &createdAtStr, &updatedAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
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
		val := parseTimeStr(lastSeenStr.String)
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

	u.CreatedAt = parseTimeStr(createdAtStr.String)
	u.UpdatedAt = parseTimeStr(updatedAtStr.String)

	return &u, nil
}

func parseTimeStr(s string) int64 {
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

func timeToDBStr(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05.000 +00:00")
}

// getDefaultPermissionsForUserType maps type to default permissions JSON
func getDefaultPermissionsForUserType(userType string) string {
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

func createSession(ctx context.Context, db *sql.DB, userID, ipAddress, userAgent, refreshToken string, expiresAt time.Time) error {
	sessionID := uuid.New().String()
	nowStr := timeToDBStr(time.Now())
	expiresStr := timeToDBStr(expiresAt)
	_, err := db.ExecContext(ctx, `INSERT INTO sessions (id, userId, ipAddress, userAgent, refreshToken, expiresAt, lastRefreshToken, lastRefreshTokenExpiresAt, createdAt, updatedAt) 
		VALUES (?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?)`,
		sessionID, userID, ipAddress, userAgent, refreshToken, expiresStr, nowStr, nowStr)
	return err
}

func deleteSessionByRefreshToken(ctx context.Context, db *sql.DB, refreshToken string) (int64, error) {
	res, err := db.ExecContext(ctx, "DELETE FROM sessions WHERE refreshToken = ?", refreshToken)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func cleanupExpiredSessions(ctx context.Context, db *sql.DB) (int64, error) {
	nowStr := timeToDBStr(time.Now())
	res, err := db.ExecContext(ctx, "DELETE FROM sessions WHERE expiresAt < ?", nowStr)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Auth Handlers (Native Go)

func handleInit(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /init")
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		hasRoot, err := HasRootUser(db)
		if err != nil {
			log.Printf("[Init] Error checking root user: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}
		if hasRoot {
			log.Printf("[Init] Attempt to init server when root user already exists")
			http.Error(w, `{"error": "Root user already exists"}`, http.StatusInternalServerError)
			return
		}

		var reqBody struct {
			NewRoot struct {
				Username string `json:"username"`
				Password string `json:"password"`
			} `json:"newRoot"`
		}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
			return
		}

		username := reqBody.NewRoot.Username
		if username == "" {
			username = "root"
		}
		password := reqBody.NewRoot.Password

		hashed, err := bcrypt.GenerateFromPassword([]byte(password), 8)
		if err != nil {
			log.Printf("[Init] Hashing failed: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		userID := uuid.New().String()
		apiToken := jwt.NewWithClaims(jwt.SigningMethodHS256, &AuthClaims{
			UserID:   userID,
			Username: username,
			Type:     "root",
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer: "audiobookshelf",
			},
		})
		tokenStr, err := apiToken.SignedString([]byte(getTokenSecret(db)))
		if err != nil {
			log.Printf("[Init] Token signing failed: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		nowStr := timeToDBStr(time.Now())
		defaultPerms := getDefaultPermissionsForUserType("root")

		_, err = db.ExecContext(r.Context(), `INSERT INTO users (id, username, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt) 
			VALUES (?, ?, 'root', ?, ?, 1, ?, '{}', '[]', ?, ?)`,
			userID, username, string(hashed), tokenStr, defaultPerms, nowStr, nowStr)
		if err != nil {
			log.Printf("[Init] Failed to create root user: %v", err)
			http.Error(w, `{"error": "Failed to create root user"}`, http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}
}

func handleLogin(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /login")
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var credentials struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&credentials); err != nil {
			http.Error(w, `{"error": "Invalid JSON body"}`, http.StatusBadRequest)
			return
		}

		user, err := getUserByUsername(r.Context(), db, credentials.Username)
		if err != nil {
			log.Printf("[Login] DB lookup failed: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		if user == nil {
			log.Printf("[Login] User not found: %s", credentials.Username)
			http.Error(w, `{"error": "Invalid username or password"}`, http.StatusUnauthorized)
			return
		}

		if !user.IsActive {
			log.Printf("[Login] User %s is inactive", user.Username)
			http.Error(w, `{"error": "User is inactive"}`, http.StatusUnauthorized)
			return
		}

		err = bcrypt.CompareHashAndPassword([]byte(user.Pash), []byte(credentials.Password))
		if err != nil {
			log.Printf("[Login] Invalid password for user %s", user.Username)
			http.Error(w, `{"error": "Invalid username or password"}`, http.StatusUnauthorized)
			return
		}

		// Generate access token (expiring)
		secret := getTokenSecret(db)
		claims := &AuthClaims{
			UserID:   user.ID,
			Username: user.Username,
			Type:     "access",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(2 * time.Hour)),
				Issuer:    "audiobookshelf",
			},
		}
		accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
		if err != nil {
			log.Printf("[Login] Failed to sign access token: %v", err)
			http.Error(w, `{"error": "Failed to login"}`, http.StatusInternalServerError)
			return
		}

		// Generate refresh token
		refreshClaims := &AuthClaims{
			UserID:   user.ID,
			Username: user.Username,
			Type:     "refresh",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
				Issuer:    "audiobookshelf",
			},
		}
		refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte(secret))
		if err != nil {
			log.Printf("[Login] Failed to sign refresh token: %v", err)
			http.Error(w, `{"error": "Failed to login"}`, http.StatusInternalServerError)
			return
		}

		// Save session
		ipAddress := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ipAddress = strings.Split(xff, ",")[0]
		}
		userAgent := r.Header.Get("User-Agent")
		expiresAt := time.Now().Add(30 * 24 * time.Hour)

		if err := createSession(r.Context(), db, user.ID, ipAddress, userAgent, refreshToken, expiresAt); err != nil {
			log.Printf("[Login] Failed to create session: %v", err)
			http.Error(w, `{"error": "Failed to login"}`, http.StatusInternalServerError)
			return
		}

		// Set Cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "refresh_token",
			Value:    refreshToken,
			Path:     "/",
			MaxAge:   30 * 24 * 60 * 60,
			HttpOnly: true,
		})

		// Return login response payload
		payload, err := getUserLoginPayload(r.Context(), db, user)
		if err != nil {
			log.Printf("[Login] Failed to build response payload: %v", err)
			http.Error(w, `{"error": "Failed to login"}`, http.StatusInternalServerError)
			return
		}

		// Include access token in response user object or payload
		userJSON := user.toOldJSONForBrowser(false)
		userJSON["accessToken"] = accessToken
		payload["user"] = userJSON

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

func handleAuthorize(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /api/authorize")
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		userVal := r.Context().Value(UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		userSess := userVal.(*UserSession)

		user, err := getUserByID(r.Context(), db, userSess.ID)
		if err != nil {
			log.Printf("[Authorize] DB lookup failed for user ID %s: %v", userSess.ID, err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}
		if user == nil {
			log.Printf("[Authorize] User not found: %s", userSess.ID)
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		payload, err := getUserLoginPayload(r.Context(), db, user)
		if err != nil {
			log.Printf("[Authorize] Failed to build response payload: %v", err)
			http.Error(w, `{"error": "Failed to authorize"}`, http.StatusInternalServerError)
			return
		}

		userJSON := user.toOldJSONForBrowser(false)
		payload["user"] = userJSON

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

func handleLogout(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /logout")
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		cookie, err := r.Cookie("refresh_token")
		if err == nil && cookie.Value != "" {
			_, _ = deleteSessionByRefreshToken(r.Context(), db, cookie.Value)
		}

		// Clear Cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "refresh_token",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
		})

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true}`))
	}
}

func handleRefresh(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /auth/refresh")
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		cookie, err := r.Cookie("refresh_token")
		if err != nil || cookie.Value == "" {
			log.Printf("[Refresh] No refresh token cookie")
			http.Error(w, `{"error": "No refresh token"}`, http.StatusBadRequest)
			return
		}

		refreshToken := cookie.Value
		secret := getTokenSecret(db)

		// Verify refresh token
		claims := &AuthClaims{}
		token, err := jwt.ParseWithClaims(refreshToken, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method")
			}
			return []byte(secret), nil
		})

		if err != nil || !token.Valid || claims.Type != "refresh" {
			log.Printf("[Refresh] Invalid refresh token: %v", err)
			http.Error(w, `{"error": "Invalid refresh token"}`, http.StatusBadRequest)
			return
		}

		// Find session in DB
		var session UserSessionDB
		var expiresAtStr string
		var lastExpiresAtStr sql.NullString
		var lastRefreshToken sql.NullString
		err = db.QueryRowContext(r.Context(), "SELECT id, userId, refreshToken, expiresAt, lastRefreshToken, lastRefreshTokenExpiresAt FROM sessions WHERE refreshToken = ? OR lastRefreshToken = ?", refreshToken, refreshToken).
			Scan(&session.ID, &session.UserID, &session.RefreshToken, &expiresAtStr, &lastRefreshToken, &lastExpiresAtStr)

		if err == sql.ErrNoRows {
			log.Printf("[Refresh] Session not found in DB")
			http.Error(w, `{"error": "Invalid refresh token"}`, http.StatusBadRequest)
			return
		} else if err != nil {
			log.Printf("[Refresh] DB error: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		session.ExpiresAt = parseTimeStr(expiresAtStr)
		if lastRefreshToken.Valid {
			session.LastRefreshToken = lastRefreshToken.String
		}
		if lastExpiresAtStr.Valid {
			session.LastRefreshTokenExpiresAt = parseTimeStr(lastExpiresAtStr.String)
		}

		user, err := getUserByID(r.Context(), db, session.UserID)
		if err != nil || user == nil || !user.IsActive {
			log.Printf("[Refresh] User inactive or not found")
			http.Error(w, `{"error": "User inactive"}`, http.StatusUnauthorized)
			return
		}

		isGracePeriod := false
		if session.RefreshToken != refreshToken {
			// Matched lastRefreshToken
			if session.LastRefreshTokenExpiresAt > time.Now().UnixNano()/int64(time.Millisecond) {
				isGracePeriod = true
				log.Printf("[Refresh] Grace period hit for user %s", user.Username)
			} else {
				log.Printf("[Refresh] Grace period expired")
				http.Error(w, `{"error": "Invalid refresh token"}`, http.StatusBadRequest)
				return
			}
		} else {
			// Matched current refreshToken, check DB expiration
			if session.ExpiresAt < time.Now().UnixNano()/int64(time.Millisecond) {
				log.Printf("[Refresh] Session expired in DB")
				db.ExecContext(r.Context(), "DELETE FROM sessions WHERE id = ?", session.ID)
				http.Error(w, `{"error": "Refresh token expired"}`, http.StatusUnauthorized)
				return
			}
		}

		newAccessTokenClaims := &AuthClaims{
			UserID:   user.ID,
			Username: user.Username,
			Type:     "access",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(2 * time.Hour)),
				Issuer:    "audiobookshelf",
			},
		}
		newAccessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, newAccessTokenClaims).SignedString([]byte(secret))
		if err != nil {
			log.Printf("[Refresh] Failed to sign new access token: %v", err)
			http.Error(w, `{"error": "Refresh failed"}`, http.StatusInternalServerError)
			return
		}

		newRefreshToken := refreshToken
		if !isGracePeriod {
			newRefreshClaims := &AuthClaims{
				UserID:   user.ID,
				Username: user.Username,
				Type:     "refresh",
				RegisteredClaims: jwt.RegisteredClaims{
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
					Issuer:    "audiobookshelf",
				},
			}
			newRefreshToken, err = jwt.NewWithClaims(jwt.SigningMethodHS256, newRefreshClaims).SignedString([]byte(secret))
			if err != nil {
				log.Printf("[Refresh] Failed to sign new refresh token: %v", err)
				http.Error(w, `{"error": "Refresh failed"}`, http.StatusInternalServerError)
				return
			}

			// Rotate tokens in DB
			nowStr := timeToDBStr(time.Now())
			expiresStr := timeToDBStr(time.Now().Add(30 * 24 * time.Hour))
			graceExpiresStr := timeToDBStr(time.Now().Add(60 * time.Second))

			_, err = db.ExecContext(r.Context(), "UPDATE sessions SET refreshToken = ?, expiresAt = ?, lastRefreshToken = ?, lastRefreshTokenExpiresAt = ?, updatedAt = ? WHERE id = ? AND refreshToken = ?",
				newRefreshToken, expiresStr, refreshToken, graceExpiresStr, nowStr, session.ID, refreshToken)
			if err != nil {
				log.Printf("[Refresh] Failed to update session in DB: %v", err)
				http.Error(w, `{"error": "Refresh failed"}`, http.StatusInternalServerError)
				return
			}

			// Update cookie
			http.SetCookie(w, &http.Cookie{
				Name:     "refresh_token",
				Value:    newRefreshToken,
				Path:     "/",
				MaxAge:   30 * 24 * 60 * 60,
				HttpOnly: true,
			})
		}

		payload, err := getUserLoginPayload(r.Context(), db, user)
		if err != nil {
			log.Printf("[Refresh] Failed to get response payload: %v", err)
			http.Error(w, `{"error": "Refresh failed"}`, http.StatusInternalServerError)
			return
		}

		userJSON := user.toOldJSONForBrowser(false)
		userJSON["accessToken"] = newAccessToken
		payload["user"] = userJSON

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

// User CRUD Handlers

func handleGetUsers(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/users")
		userSess := r.Context().Value(UserContextKey).(*UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		hideRootToken := userSess.Type != "root"

		rows, err := db.QueryContext(r.Context(), "SELECT id, username, email, pash, type, token, isActive, isLocked, lastSeen, permissions, bookmarks, extraData, createdAt, updatedAt FROM users ORDER BY username ASC")
		if err != nil {
			log.Printf("[Users] DB Query failed: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
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
			var pashStr, tokenStr sql.NullString

			err := rows.Scan(&u.ID, &u.Username, &email, &pashStr, &u.Type, &tokenStr, &isActiveInt, &isLockedInt, &lastSeenStr, &permsStr, &bookmarksStr, &extraDataStr, &createdAtStr, &updatedAtStr)
			if err != nil {
				log.Printf("[Users] Failed to scan user: %v", err)
				continue
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
				val := parseTimeStr(lastSeenStr.String)
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
			u.CreatedAt = parseTimeStr(createdAtStr.String)
			u.UpdatedAt = parseTimeStr(updatedAtStr.String)

			usersJSON = append(usersJSON, u.toOldJSONForBrowser(hideRootToken))
		}
		if err := rows.Err(); err != nil {
			log.Printf("[Users] Users query iteration error: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"users": usersJSON,
		})
	}
}

func handleGetOnlineUsers(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/users/online")
		userSess := r.Context().Value(UserContextKey).(*UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var online []OnlineUser
		if SocketAuth != nil {
			online = SocketAuth.GetUsersOnline()
		} else {
			online = []OnlineUser{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"usersOnline":  online,
			"openSessions": []interface{}{},
		})
	}
}

func handleUserCRUD(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(UserContextKey).(*UserSession)

		if r.Method == http.MethodPost && r.URL.Path == "/api/users" {
			log.Printf("[Go] POST /api/users")
			if userSess.Type != "root" && userSess.Type != "admin" {
				http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
				return
			}

			var body struct {
				Username            string                  `json:"username"`
				Password            string                  `json:"password"`
				Email               string                  `json:"email"`
				Type                string                  `json:"type"`
				IsActive            bool                    `json:"isActive"`
				Permissions         UserPermissionsDetailed `json:"permissions"`
				LibrariesAccessible []string                `json:"librariesAccessible"`
				ItemTagsSelected    []string                `json:"itemTagsSelected"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
				return
			}

			if body.Username == "" || body.Password == "" {
				http.Error(w, `{"error": "Username and password are required"}`, http.StatusBadRequest)
				return
			}

			exists, err := checkUserExistsWithUsername(r.Context(), db, body.Username)
			if err != nil || exists {
				http.Error(w, `{"error": "Username already taken"}`, http.StatusBadRequest)
				return
			}

			hashed, err := bcrypt.GenerateFromPassword([]byte(body.Password), 8)
			if err != nil {
				http.Error(w, `{"error": "Failed to create user"}`, http.StatusInternalServerError)
				return
			}

			userID := uuid.New().String()
			secret := getTokenSecret(db)
			apiToken := jwt.NewWithClaims(jwt.SigningMethodHS256, &AuthClaims{
				UserID:   userID,
				Username: body.Username,
				Type:     body.Type,
				RegisteredClaims: jwt.RegisteredClaims{
					Issuer: "audiobookshelf",
				},
			})
			tokenStr, _ := apiToken.SignedString([]byte(secret))

			userType := body.Type
			if userType == "" {
				userType = "user"
			}

			perms := map[string]interface{}{
				"download":                  true,
				"accessExplicitContent":     false,
				"accessAllLibraries":        true,
				"librariesAccessible":       []string{},
				"accessAllTags":             true,
				"itemTagsSelected":          []string{},
				"selectedTagsNotAccessible": false,
			}
			if body.Permissions.Download != nil {
				perms["download"] = *body.Permissions.Download
			}
			if body.Permissions.AccessExplicitContent != nil {
				perms["accessExplicitContent"] = *body.Permissions.AccessExplicitContent
			}
			if body.Permissions.AccessAllLibraries != nil {
				perms["accessAllLibraries"] = *body.Permissions.AccessAllLibraries
			}
			if len(body.Permissions.LibrariesAccessible) > 0 {
				perms["librariesAccessible"] = body.Permissions.LibrariesAccessible
			} else if len(body.LibrariesAccessible) > 0 {
				perms["librariesAccessible"] = body.LibrariesAccessible
			}
			if body.Permissions.AccessAllTags != nil {
				perms["accessAllTags"] = *body.Permissions.AccessAllTags
			}
			if len(body.Permissions.ItemTagsSelected) > 0 {
				perms["itemTagsSelected"] = body.Permissions.ItemTagsSelected
			} else if len(body.ItemTagsSelected) > 0 {
				perms["itemTagsSelected"] = body.ItemTagsSelected
			}
			if body.Permissions.SelectedTagsNotAccessible != nil {
				perms["selectedTagsNotAccessible"] = *body.Permissions.SelectedTagsNotAccessible
			}

			permsBytes, _ := json.Marshal(perms)
			nowStr := timeToDBStr(time.Now())

			var emailVal interface{} = nil
			if body.Email != "" {
				emailVal = body.Email
			}

			_, err = db.ExecContext(r.Context(), `INSERT INTO users (id, username, email, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt) 
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, '{}', '[]', ?, ?)`,
				userID, body.Username, emailVal, userType, string(hashed), tokenStr, func() int {
					if body.IsActive {
						return 1
					}
					return 0
				}(), string(permsBytes), nowStr, nowStr)

			if err != nil {
				log.Printf("[User Create] DB Error: %v", err)
				http.Error(w, `{"error": "Failed to save user"}`, http.StatusInternalServerError)
				return
			}

			savedUser, err := getUserByID(r.Context(), db, userID)
			if err != nil || savedUser == nil {
				http.Error(w, `{"error": "Internal Error"}`, http.StatusInternalServerError)
				return
			}

			userJSON := savedUser.toOldJSONForBrowser(userSess.Type != "root")

			if SocketAuth != nil {
				SocketAuth.BroadcastToAdmins("user_added", userJSON)
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"user": userJSON,
			})
			return
		}

		subPath := strings.TrimPrefix(r.URL.Path, "/api/users/")
		if subPath == "" || strings.Contains(subPath, "/") {
			http.NotFound(w, r)
			return
		}

		targetUserID := subPath
		isUnlinkRoute := false
		if strings.HasSuffix(targetUserID, "/openid-unlink") {
			targetUserID = strings.TrimSuffix(targetUserID, "/openid-unlink")
			isUnlinkRoute = true
		}

		if isUnlinkRoute {
			log.Printf("[Go] PATCH /api/users/%s/openid-unlink", targetUserID)
			if userSess.Type != "root" && userSess.Type != "admin" && userSess.ID != targetUserID {
				http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
				return
			}

			targetUser, err := getUserByID(r.Context(), db, targetUserID)
			if err != nil || targetUser == nil {
				http.Error(w, `{"error": "User not found"}`, http.StatusNotFound)
				return
			}

			var extra map[string]interface{}
			if len(targetUser.ExtraData) > 0 {
				json.Unmarshal(targetUser.ExtraData, &extra)
			}
			if extra == nil {
				extra = make(map[string]interface{})
			}
			delete(extra, "authOpenIDSub")
			newExtraBytes, _ := json.Marshal(extra)

			_, err = db.ExecContext(r.Context(), "UPDATE users SET extraData = ?, updatedAt = ? WHERE id = ?", string(newExtraBytes), timeToDBStr(time.Now()), targetUserID)
			if err != nil {
				http.Error(w, `{"error": "Failed to update user"}`, http.StatusInternalServerError)
				return
			}

			targetUser.ExtraData = newExtraBytes
			userJSON := targetUser.toOldJSONForBrowser(userSess.Type != "root")
			if SocketAuth != nil {
				SocketAuth.BroadcastToUser(targetUserID, "user_updated", userJSON)
			}

			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodGet {
			log.Printf("[Go] GET /api/users/%s", targetUserID)
			if userSess.Type != "root" && userSess.Type != "admin" && userSess.ID != targetUserID {
				http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
				return
			}

			targetUser, err := getUserByID(r.Context(), db, targetUserID)
			if err != nil || targetUser == nil {
				http.Error(w, `{"error": "User not found"}`, http.StatusNotFound)
				return
			}

			userJSON := targetUser.toOldJSONForBrowser(userSess.Type != "root")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(userJSON)
			return
		}

		if r.Method == http.MethodPatch {
			log.Printf("[Go] PATCH /api/users/%s", targetUserID)
			if userSess.Type != "root" && userSess.Type != "admin" {
				http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
				return
			}

			targetUser, err := getUserByID(r.Context(), db, targetUserID)
			if err != nil || targetUser == nil {
				http.Error(w, `{"error": "User not found"}`, http.StatusNotFound)
				return
			}

			if targetUser.Type == "root" && userSess.Type != "root" {
				http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
				return
			}

			var body struct {
				Username            *string                 `json:"username"`
				Password            *string                 `json:"password"`
				Email               *string                 `json:"email"`
				Type                *string                 `json:"type"`
				IsActive            *bool                   `json:"isActive"`
				Permissions         *map[string]interface{} `json:"permissions"`
				LibrariesAccessible []string                `json:"librariesAccessible"`
				ItemTagsSelected    []string                `json:"itemTagsSelected"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
				return
			}

			hasUpdates := false
			shouldUpdateToken := false

			if body.Username != nil && *body.Username != targetUser.Username {
				exists, err := checkUserExistsWithUsername(r.Context(), db, *body.Username)
				if err != nil || exists {
					http.Error(w, `{"error": "Username already taken"}`, http.StatusBadRequest)
					return
				}
				targetUser.Username = *body.Username
				shouldUpdateToken = true
				hasUpdates = true
			}

			if body.Password != nil && *body.Password != "" {
				hashed, err := bcrypt.GenerateFromPassword([]byte(*body.Password), 8)
				if err != nil {
					http.Error(w, `{"error": "Failed to update password"}`, http.StatusInternalServerError)
					return
				}
				targetUser.Pash = string(hashed)
				hasUpdates = true
			}

			if body.Email != nil {
				if *body.Email == "" {
					targetUser.Email = nil
				} else {
					targetUser.Email = body.Email
				}
				hasUpdates = true
			}

			if body.Type != nil && targetUser.Type != "root" {
				targetUser.Type = *body.Type
				hasUpdates = true
			}

			if body.IsActive != nil {
				targetUser.IsActive = *body.IsActive
				hasUpdates = true
			}

			if body.Permissions != nil || len(body.LibrariesAccessible) > 0 || len(body.ItemTagsSelected) > 0 {
				var currentPerms map[string]interface{}
				json.Unmarshal(targetUser.Permissions, &currentPerms)
				if currentPerms == nil {
					currentPerms = make(map[string]interface{})
				}

				if body.Permissions != nil {
					for k, v := range *body.Permissions {
						if k == "librariesAccessible" || k == "itemTagsSelected" {
							continue
						}
						currentPerms[k] = v
					}
				}

				if len(body.LibrariesAccessible) > 0 {
					currentPerms["librariesAccessible"] = body.LibrariesAccessible
				}
				if len(body.ItemTagsSelected) > 0 {
					currentPerms["itemTagsSelected"] = body.ItemTagsSelected
				}

				newPermsBytes, _ := json.Marshal(currentPerms)
				targetUser.Permissions = newPermsBytes
				hasUpdates = true
			}

			if hasUpdates {
				if shouldUpdateToken {
					secret := getTokenSecret(db)
					apiToken := jwt.NewWithClaims(jwt.SigningMethodHS256, &AuthClaims{
						UserID:   targetUser.ID,
						Username: targetUser.Username,
						Type:     targetUser.Type,
						RegisteredClaims: jwt.RegisteredClaims{
							Issuer: "audiobookshelf",
						},
					})
					targetUser.Token, _ = apiToken.SignedString([]byte(secret))
				}

				if shouldUpdateToken {
					db.ExecContext(r.Context(), "DELETE FROM sessions WHERE userId = ?", targetUser.ID)
				}

				nowStr := timeToDBStr(time.Now())
				var emailVal interface{} = nil
				if targetUser.Email != nil {
					emailVal = *targetUser.Email
				}
				_, err = db.ExecContext(r.Context(), `UPDATE users SET username = ?, email = ?, type = ?, pash = ?, token = ?, isActive = ?, permissions = ?, updatedAt = ? WHERE id = ?`,
					targetUser.Username, emailVal, targetUser.Type, targetUser.Pash, targetUser.Token, func() int {
						if targetUser.IsActive {
							return 1
						}
						return 0
					}(), string(targetUser.Permissions), nowStr, targetUser.ID)

				if err != nil {
					log.Printf("[User Update] DB Error: %v", err)
					http.Error(w, `{"error": "Failed to update user"}`, http.StatusInternalServerError)
					return
				}
			}

			updatedUser, _ := getUserByID(r.Context(), db, targetUserID)
			userJSON := updatedUser.toOldJSONForBrowser(userSess.Type != "root")

			if SocketAuth != nil {
				SocketAuth.BroadcastToUser(targetUserID, "user_updated", userJSON)
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"user":    userJSON,
			})
			return
		}

		if r.Method == http.MethodDelete {
			log.Printf("[Go] DELETE /api/users/%s", targetUserID)
			if userSess.Type != "root" && userSess.Type != "admin" {
				http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
				return
			}
			if targetUserID == "root" {
				http.Error(w, `{"error": "Cannot delete root user"}`, http.StatusBadRequest)
				return
			}
			if targetUserID == userSess.ID {
				http.Error(w, `{"error": "Cannot delete self"}`, http.StatusBadRequest)
				return
			}

			targetUser, err := getUserByID(r.Context(), db, targetUserID)
			if err != nil || targetUser == nil {
				http.Error(w, `{"error": "User not found"}`, http.StatusNotFound)
				return
			}

			userJSON := targetUser.toOldJSONForBrowser(userSess.Type != "root")

			tx, err := db.BeginTx(r.Context(), nil)
			if err != nil {
				http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
				return
			}
			defer tx.Rollback()

			_, _ = tx.ExecContext(r.Context(), "DELETE FROM playlists WHERE userId = ?", targetUserID)
			_, _ = tx.ExecContext(r.Context(), "DELETE FROM sessions WHERE userId = ?", targetUserID)
			_, _ = tx.ExecContext(r.Context(), "DELETE FROM mediaProgress WHERE userId = ?", targetUserID)
			_, _ = tx.ExecContext(r.Context(), "DELETE FROM users WHERE id = ?", targetUserID)

			if err := tx.Commit(); err != nil {
				http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
				return
			}

			if SocketAuth != nil {
				SocketAuth.BroadcastToAdmins("user_removed", userJSON)
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
			})
			return
		}

		http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
	}
}

func getUserLoginPayload(ctx context.Context, db *sql.DB, user *User) (map[string]interface{}, error) {
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

	// 2. Get libraries filtered by user's access
	userSess := &UserSession{
		ID:   user.ID,
		Type: user.Type,
	}
	parsePermissions(sql.NullString{String: string(user.Permissions), Valid: len(user.Permissions) > 0}, userSess)

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

	payload := map[string]interface{}{
		"serverSettings": browserSettings,
		"libraries":      filteredLibs,
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
			var pashStr, tokenStr sql.NullString

			err := rows.Scan(&u.ID, &u.Username, &email, &pashStr, &u.Type, &tokenStr, &isActiveInt, &isLockedInt, &lastSeenStr, &permsStr, &bookmarksStr, &extraDataStr, &createdAtStr, &updatedAtStr)
			if err != nil {
				log.Printf("[Users] Failed to scan user for login payload: %v", err)
				continue
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
				val := parseTimeStr(lastSeenStr.String)
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
			u.CreatedAt = parseTimeStr(createdAtStr.String)
			u.UpdatedAt = parseTimeStr(updatedAtStr.String)

			usersJSON = append(usersJSON, u.toOldJSONForBrowser(hideRootToken))
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		payload["users"] = usersJSON
	}

	return payload, nil
}

func findUserFromOpenIdUserInfo(ctx context.Context, db *sql.DB, userinfo map[string]interface{}, matchBy string) (*User, error) {
	sub, _ := userinfo["sub"].(string)
	if sub == "" {
		return nil, fmt.Errorf("invalid userinfo, no sub")
	}

	var u *User
	row := db.QueryRowContext(ctx, "SELECT id, username, email, pash, type, token, isActive, isLocked, lastSeen, permissions, bookmarks, extraData, createdAt, updatedAt FROM users WHERE json_extract(extraData, '$.authOpenIDSub') = ?", sub)
	u, err := scanUser(row)
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
			u, err = scanUser(row)
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
			u, err = scanUser(row)
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

func createUserFromOpenIdUserInfo(ctx context.Context, db *sql.DB, userinfo map[string]interface{}, tokenSecret string) (*User, error) {
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

	claims := jwt.MapClaims{
		"id":       userId,
		"username": username,
		"exp":      time.Now().Add(30 * 24 * time.Hour).Unix(),
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
	perms := getDefaultPermissionsForUserType("user")
	nowStr := timeToDBStr(time.Now())

	var emailStr sql.NullString
	if emailVal != nil {
		emailStr.String = *emailVal
		emailStr.Valid = true
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO users (id, username, email, pash, type, token, isActive, isLocked, permissions, bookmarks, extraData, createdAt, updatedAt)
		VALUES (?, ?, ?, NULL, 'user', ?, 1, 0, ?, '[]', ?, ?, ?)`,
		userId, username, emailStr, tokenString, perms, string(extraBytes), nowStr, nowStr)
	if err != nil {
		return nil, err
	}

	return &User{
		ID:          userId,
		Username:    username,
		Email:       emailVal,
		Type:        "user",
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
