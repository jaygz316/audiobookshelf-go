package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	isocket "audiobookshelf/internal/socket"
	"audiobookshelf/internal/utils"
)

// Auth Handlers (Native Go)

func handleInit(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("[Go] POST /init")
		if db == nil {
			http.Error(w, `{"error": "Database not connected"}`, http.StatusInternalServerError)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		hasRoot, err := idb.HasRootUser(db)
		if err != nil {
			log.Errorf("[Init] Error checking root user: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}
		if hasRoot {
			log.Warnf("[Init] Attempt to init server when root user already exists")
			http.Error(w, `{"error": "Root user already exists"}`, http.StatusForbidden)
			return
		}

		var reqBody struct {
			NewRoot struct {
				Username string `json:"username"`
				Password string `json:"password"`
			} `json:"newRoot"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1048576)
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
			log.Errorf("[Init] Hashing failed: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		userID := uuid.New().String()
		apiToken := jwt.NewWithClaims(jwt.SigningMethodHS256, &core.AuthClaims{
			UserID:   userID,
			Username: username,
			Type:     "root",
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer: "audiobookshelf",
			},
		})
		tokenStr, err := apiToken.SignedString([]byte(getTokenSecret(db)))
		if err != nil {
			log.Errorf("[Init] Token signing failed: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		nowStr := idb.TimeToDBStr(time.Now())
		defaultPerms := idb.GetDefaultPermissionsForUserType("root")

		_, err = db.ExecContext(r.Context(), `INSERT INTO users (id, username, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt) 
			VALUES (?, ?, 'root', ?, ?, 1, ?, '{}', '[]', ?, ?)`,
			userID, username, string(hashed), tokenStr, defaultPerms, nowStr, nowStr)
		if err != nil {
			log.Errorf("[Init] Failed to create root user: %v", err)
			http.Error(w, `{"error": "Failed to create root user"}`, http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}
}

func handleLogin(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("[Go] POST /login")
		if db == nil {
			http.Error(w, `{"error": "Database not connected"}`, http.StatusInternalServerError)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		if db == nil {
			log.Warnf("[Login] Database not available")
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error": "Server is not initialized yet. Please wait for the database to be ready."}`, http.StatusServiceUnavailable)
			return
		}

		var credentials struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1048576)
		if err := json.NewDecoder(r.Body).Decode(&credentials); err != nil {
			http.Error(w, `{"error": "Invalid JSON body"}`, http.StatusBadRequest)
			return
		}

		user, err := idb.GetUserFullByUsername(r.Context(), db, credentials.Username)
		if err != nil {
			log.Errorf("[Login] DB lookup failed: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		if user == nil {
			log.Warnf("[Login] idb.User not found: %s", credentials.Username)
			http.Error(w, `{"error": "Invalid username or password"}`, http.StatusUnauthorized)
			return
		}

		if !user.IsActive {
			log.Warnf("[Login] idb.User %s is inactive", user.Username)
			http.Error(w, `{"error": "idb.User is inactive"}`, http.StatusUnauthorized)
			return
		}

		err = bcrypt.CompareHashAndPassword([]byte(user.Pash), []byte(credentials.Password))
		if err != nil {
			log.Warnf("[Login] Invalid password for user %s", user.Username)
			http.Error(w, `{"error": "Invalid username or password"}`, http.StatusUnauthorized)
			return
		}

		// Generate access token (expiring)
		secret := getTokenSecret(db)
		claims := &core.AuthClaims{
			UserID:   user.ID,
			Username: user.Username,
			Type:     "access",
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        uuid.New().String(),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(2 * time.Hour)),
				Issuer:    "audiobookshelf",
			},
		}
		accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
		if err != nil {
			log.Errorf("[Login] Failed to sign access token: %v", err)
			http.Error(w, `{"error": "Failed to login"}`, http.StatusInternalServerError)
			return
		}

		// Generate refresh token
		refreshClaims := &core.AuthClaims{
			UserID:   user.ID,
			Username: user.Username,
			Type:     "refresh",
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        uuid.New().String(),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
				Issuer:    "audiobookshelf",
			},
		}
		refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte(secret))
		if err != nil {
			log.Errorf("[Login] Failed to sign refresh token: %v", err)
			http.Error(w, `{"error": "Failed to login"}`, http.StatusInternalServerError)
			return
		}

		// Save session
		ipAddress := utils.GetClientIP(r)
		userAgent := r.Header.Get("User-Agent")
		expiresAt := time.Now().Add(30 * 24 * time.Hour)

		if err := idb.CreateSession(r.Context(), db, user.ID, ipAddress, userAgent, refreshToken, expiresAt); err != nil {
			log.Errorf("[Login] Failed to create session: %v", err)
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
		payload, err := idb.GetUserLoginPayload(r.Context(), db, user)
		if err != nil {
			log.Errorf("[Login] Failed to build response payload: %v", err)
			http.Error(w, `{"error": "Failed to login"}`, http.StatusInternalServerError)
			return
		}

		// Include access token in response user object or payload
		userJSON := user.ToOldJSONForBrowser(false)
		userJSON["accessToken"] = accessToken
		payload["user"] = userJSON

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

func handleAuthorize(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("[Go] /api/authorize")
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		userSess := userVal.(*core.UserSession)

		user, err := idb.GetUserFullByID(r.Context(), db, userSess.ID)
		if err != nil {
			log.Errorf("[Authorize] DB lookup failed for user ID %s: %v", userSess.ID, err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}
		if user == nil {
			log.Warnf("[Authorize] idb.User not found: %s", userSess.ID)
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		payload, err := idb.GetUserLoginPayload(r.Context(), db, user)
		if err != nil {
			log.Errorf("[Authorize] Failed to build response payload: %v", err)
			http.Error(w, `{"error": "Failed to authorize"}`, http.StatusInternalServerError)
			return
		}

		userJSON := user.ToOldJSONForBrowser(false)
		payload["user"] = userJSON

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

func handleLogout(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("[Go] POST /logout")
		if db == nil {
			http.Error(w, `{"error": "Database not connected"}`, http.StatusInternalServerError)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		cookie, err := r.Cookie("refresh_token")
		if err == nil && cookie.Value != "" {
			_, _ = idb.DeleteSessionByRefreshToken(r.Context(), db, cookie.Value)
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
		log.Info("[Go] POST /auth/refresh")
		if db == nil {
			http.Error(w, `{"error": "Database not connected"}`, http.StatusInternalServerError)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		cookie, err := r.Cookie("refresh_token")
		if err != nil || cookie.Value == "" {
			log.Warnf("[Refresh] No refresh token cookie")
			http.Error(w, `{"error": "No refresh token"}`, http.StatusBadRequest)
			return
		}

		refreshToken := cookie.Value
		secret := getTokenSecret(db)

		// Verify refresh token
		claims := &core.AuthClaims{}
		token, err := jwt.ParseWithClaims(refreshToken, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method")
			}
			return []byte(secret), nil
		})

		if err != nil || !token.Valid || claims.Type != "refresh" {
			log.Warnf("[Refresh] Invalid refresh token: %v", err)
			http.Error(w, `{"error": "Invalid refresh token"}`, http.StatusBadRequest)
			return
		}

		// Find session in DB
		var session idb.UserSessionDB
		var expiresAtStr string
		var lastExpiresAtStr sql.NullString
		var lastRefreshToken sql.NullString
		err = db.QueryRowContext(r.Context(), "SELECT id, userId, refreshToken, expiresAt, lastRefreshToken, lastRefreshTokenExpiresAt FROM sessions WHERE refreshToken = ? OR lastRefreshToken = ?", refreshToken, refreshToken).
			Scan(&session.ID, &session.UserID, &session.RefreshToken, &expiresAtStr, &lastRefreshToken, &lastExpiresAtStr)

		if err == sql.ErrNoRows {
			log.Warnf("[Refresh] Session not found in DB")
			http.Error(w, `{"error": "Invalid refresh token"}`, http.StatusBadRequest)
			return
		} else if err != nil {
			log.Errorf("[Refresh] DB error: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		session.ExpiresAt = idb.ParseTimeStr(expiresAtStr)
		if lastRefreshToken.Valid {
			session.LastRefreshToken = lastRefreshToken.String
		}
		if lastExpiresAtStr.Valid {
			session.LastRefreshTokenExpiresAt = idb.ParseTimeStr(lastExpiresAtStr.String)
		}

		user, err := idb.GetUserFullByID(r.Context(), db, session.UserID)
		if err != nil || user == nil || !user.IsActive {
			log.Warnf("[Refresh] idb.User inactive or not found")
			http.Error(w, `{"error": "idb.User inactive"}`, http.StatusUnauthorized)
			return
		}

		isGracePeriod := false
		if session.RefreshToken != refreshToken {
			// Matched lastRefreshToken
			if session.LastRefreshTokenExpiresAt > time.Now().UnixNano()/int64(time.Millisecond) {
				isGracePeriod = true
				log.Infof("[Refresh] Grace period hit for user %s", user.Username)
			} else {
				log.Warnf("[Refresh] Grace period expired")
				http.Error(w, `{"error": "Invalid refresh token"}`, http.StatusBadRequest)
				return
			}
		} else {
			// Matched current refreshToken, check DB expiration
			if session.ExpiresAt < time.Now().UnixNano()/int64(time.Millisecond) {
				log.Warnf("[Refresh] Session expired in DB")
				db.ExecContext(r.Context(), "DELETE FROM sessions WHERE id = ?", session.ID)
				http.Error(w, `{"error": "Refresh token expired"}`, http.StatusUnauthorized)
				return
			}
		}

		newAccessTokenClaims := &core.AuthClaims{
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
			log.Errorf("[Refresh] Failed to sign new access token: %v", err)
			http.Error(w, `{"error": "Refresh failed"}`, http.StatusInternalServerError)
			return
		}

		newRefreshToken := refreshToken
		if !isGracePeriod {
			newRefreshClaims := &core.AuthClaims{
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
				log.Errorf("[Refresh] Failed to sign new refresh token: %v", err)
				http.Error(w, `{"error": "Refresh failed"}`, http.StatusInternalServerError)
				return
			}

			// Rotate tokens in DB
			nowStr := idb.TimeToDBStr(time.Now())
			expiresStr := idb.TimeToDBStr(time.Now().Add(30 * 24 * time.Hour))
			graceExpiresStr := idb.TimeToDBStr(time.Now().Add(60 * time.Second))

			_, err = db.ExecContext(r.Context(), "UPDATE sessions SET refreshToken = ?, expiresAt = ?, lastRefreshToken = ?, lastRefreshTokenExpiresAt = ?, updatedAt = ? WHERE id = ? AND refreshToken = ?",
				newRefreshToken, expiresStr, refreshToken, graceExpiresStr, nowStr, session.ID, refreshToken)
			if err != nil {
				log.Errorf("[Refresh] Failed to update session in DB: %v", err)
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

		payload, err := idb.GetUserLoginPayload(r.Context(), db, user)
		if err != nil {
			log.Errorf("[Refresh] Failed to get response payload: %v", err)
			http.Error(w, `{"error": "Refresh failed"}`, http.StatusInternalServerError)
			return
		}

		userJSON := user.ToOldJSONForBrowser(false)
		userJSON["accessToken"] = newAccessToken
		payload["user"] = userJSON

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

// idb.User CRUD Handlers

func handleGetUsers(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("[Go] GET /api/users")
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		hideRootToken := userSess.Type != "root"

		rows, err := db.QueryContext(r.Context(), "SELECT id, username, email, pash, type, token, isActive, isLocked, lastSeen, permissions, bookmarks, extraData, createdAt, updatedAt FROM users ORDER BY username ASC")
		if err != nil {
			log.Errorf("[Users] DB Query failed: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var usersJSON []map[string]interface{}
		for rows.Next() {
			var u idb.User
			var email sql.NullString
			var lastSeenStr sql.NullString
			var isActiveInt, isLockedInt sql.NullInt64
			var permsStr, bookmarksStr, extraDataStr sql.NullString
			var createdAtStr, updatedAtStr sql.NullString
			var pashStr, tokenStr, typeStr sql.NullString

			err := rows.Scan(&u.ID, &u.Username, &email, &pashStr, &typeStr, &tokenStr, &isActiveInt, &isLockedInt, &lastSeenStr, &permsStr, &bookmarksStr, &extraDataStr, &createdAtStr, &updatedAtStr)
			if err != nil {
				log.Errorf("[Users] Failed to scan user: %v", err)
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
				val := idb.ParseTimeStr(lastSeenStr.String)
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
			u.CreatedAt = idb.ParseTimeStr(createdAtStr.String)
			u.UpdatedAt = idb.ParseTimeStr(updatedAtStr.String)

			usersJSON = append(usersJSON, u.ToOldJSONForBrowser(hideRootToken))
		}
		if err := rows.Err(); err != nil {
			log.Errorf("[Users] Users query iteration error: %v", err)
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
		log.Info("[Go] GET /api/users/online")
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var online []isocket.OnlineUser
		if isocket.GlobalAuth != nil {
			online = isocket.GlobalAuth.GetUsersOnline()
		}
		if online == nil {
			online = []isocket.OnlineUser{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(online)
	}
}

func handleUserCRUD(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)

		pathWithoutPrefix := utils.TrimAPIPath(r.URL.Path, "/api/users")
		if r.Method == http.MethodPost && (pathWithoutPrefix == "" || pathWithoutPrefix == "/") {
			log.Info("[Go] POST /api/users")
			if userSess.Type != "root" && userSess.Type != "admin" {
				http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
				return
			}

			var body struct {
				Username            string                      `json:"username"`
				Password            string                      `json:"password"`
				Email               string                      `json:"email"`
				Type                string                      `json:"type"`
				IsActive            *bool                       `json:"isActive"`
				Permissions         idb.UserPermissionsDetailed `json:"permissions"`
				LibrariesAccessible []string                    `json:"librariesAccessible"`
				ItemTagsSelected    []string                    `json:"itemTagsSelected"`
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1048576)
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
				return
			}

			if body.Username == "" || body.Password == "" {
				http.Error(w, `{"error": "Username and password are required"}`, http.StatusBadRequest)
				return
			}

			exists, err := idb.CheckUserExistsWithUsername(r.Context(), db, body.Username)
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
			apiToken := jwt.NewWithClaims(jwt.SigningMethodHS256, &core.AuthClaims{
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
			nowStr := idb.TimeToDBStr(time.Now())

			var emailVal interface{} = nil
			if body.Email != "" {
				emailVal = body.Email
			}

			_, err = db.ExecContext(r.Context(), `INSERT INTO users (id, username, email, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt) 
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, '{}', '[]', ?, ?)`,
				userID, body.Username, emailVal, userType, string(hashed), tokenStr, func() int {
					if body.IsActive == nil || *body.IsActive {
						return 1
					}
					return 0
				}(), string(permsBytes), nowStr, nowStr)

			if err != nil {
				log.Errorf("[idb.User Create] DB Error: %v", err)
				http.Error(w, `{"error": "Failed to save user"}`, http.StatusInternalServerError)
				return
			}

			savedUser, err := idb.GetUserFullByID(r.Context(), db, userID)
			if err != nil || savedUser == nil {
				http.Error(w, `{"error": "Internal Error"}`, http.StatusInternalServerError)
				return
			}

			userJSON := savedUser.ToOldJSONForBrowser(userSess.Type != "root")

			if isocket.GlobalAuth != nil {
				isocket.GlobalAuth.BroadcastToAdmins("user_added", userJSON)
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"user": userJSON,
			})
			return
		}

		subPath := strings.TrimPrefix(pathWithoutPrefix, "/")
		if subPath == "" {
			http.NotFound(w, r)
			return
		}

		parts := strings.Split(subPath, "/")
		if len(parts) == 2 && parts[1] == "listening-stats" {
			if r.Method == http.MethodGet {
				targetUserID := parts[0]
				log.Infof("[Go] GET /api/users/%s/listening-stats", targetUserID)
				if userSess.Type != "root" && userSess.Type != "admin" && userSess.ID != targetUserID {
					http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
					return
				}
				stats, err := getUserListeningStats(db, targetUserID)
				if err != nil {
					log.Errorf("[Listening Stats] Failed to query stats: %v", err)
					http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(stats)
				return
			}
		} else if len(parts) == 2 && parts[1] == "listening-sessions" {
			if r.Method == http.MethodGet {
				targetUserID := parts[0]
				log.Infof("[Go] GET /api/users/%s/listening-sessions", targetUserID)
				if userSess.Type != "root" && userSess.Type != "admin" && userSess.ID != targetUserID {
					http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
					return
				}
				page := 0
				if pVal := r.URL.Query().Get("page"); pVal != "" {
					if p, err := strconv.Atoi(pVal); err == nil {
						page = p
					}
				}
				itemsPerPage := 10
				if limitVal := r.URL.Query().Get("itemsPerPage"); limitVal != "" {
					if limit, err := strconv.Atoi(limitVal); err == nil {
						itemsPerPage = limit
					}
				}
				sessions, err := handleGetUserListeningSessions(db, targetUserID, page, itemsPerPage)
				if err != nil {
					log.Errorf("[Listening Sessions] Failed to query sessions: %v", err)
					http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(sessions)
				return
			}
		} else if len(parts) == 2 && parts[1] == "sessions" {
			if r.Method == http.MethodGet {
				targetUserID := parts[0]
				log.Infof("[Go] GET /api/users/%s/sessions", targetUserID)
				handleGetUserLoginSessions(db, targetUserID)(w, r)
				return
			}
		} else if len(parts) == 3 && parts[1] == "sessions" {
			if r.Method == http.MethodDelete {
				targetUserID := parts[0]
				sessionID := parts[2]
				log.Infof("[Go] DELETE /api/users/%s/sessions/%s", targetUserID, sessionID)
				handleDeleteUserLoginSession(db, targetUserID, sessionID)(w, r)
				return
			}
		}

		targetUserID := subPath
		isUnlinkRoute := false
		if strings.HasSuffix(targetUserID, "/openid-unlink") {
			targetUserID = strings.TrimSuffix(targetUserID, "/openid-unlink")
			isUnlinkRoute = true
		}

		if isUnlinkRoute {
			log.Infof("[Go] PATCH /api/users/%s/openid-unlink", targetUserID)
			if userSess.Type != "root" && userSess.Type != "admin" && userSess.ID != targetUserID {
				http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
				return
			}

			targetUser, err := idb.GetUserFullByID(r.Context(), db, targetUserID)
			if err != nil || targetUser == nil {
				http.Error(w, `{"error": "idb.User not found"}`, http.StatusNotFound)
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

			_, err = db.ExecContext(r.Context(), "UPDATE users SET extraData = ?, updatedAt = ? WHERE id = ?", string(newExtraBytes), idb.TimeToDBStr(time.Now()), targetUserID)
			if err != nil {
				http.Error(w, `{"error": "Failed to update user"}`, http.StatusInternalServerError)
				return
			}

			targetUser.ExtraData = newExtraBytes
			userJSON := targetUser.ToOldJSONForBrowser(userSess.Type != "root")
			if isocket.GlobalAuth != nil {
				isocket.GlobalAuth.BroadcastToUser(targetUserID, "user_updated", userJSON)
			}

			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodGet {
			log.Infof("[Go] GET /api/users/%s", targetUserID)
			if userSess.Type != "root" && userSess.Type != "admin" && userSess.ID != targetUserID {
				http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
				return
			}

			targetUser, err := idb.GetUserFullByID(r.Context(), db, targetUserID)
			if err != nil || targetUser == nil {
				http.Error(w, `{"error": "idb.User not found"}`, http.StatusNotFound)
				return
			}

			userJSON := targetUser.ToOldJSONForBrowser(userSess.Type != "root")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(userJSON)
			return
		}

		if r.Method == http.MethodPatch {
			log.Infof("[Go] PATCH /api/users/%s", targetUserID)
			if userSess.Type != "root" && userSess.Type != "admin" {
				http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
				return
			}

			targetUser, err := idb.GetUserFullByID(r.Context(), db, targetUserID)
			if err != nil || targetUser == nil {
				http.Error(w, `{"error": "idb.User not found"}`, http.StatusNotFound)
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
				ItemTagsSelected    *[]string               `json:"itemTagsSelected"`
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1048576)
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
				return
			}

			hasUpdates := false
			shouldUpdateToken := false

			if body.Username != nil && *body.Username != targetUser.Username {
				exists, err := idb.CheckUserExistsWithUsername(r.Context(), db, *body.Username)
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

			if body.Permissions != nil || len(body.LibrariesAccessible) > 0 || body.ItemTagsSelected != nil {
				var currentPerms map[string]interface{}
				json.Unmarshal(targetUser.Permissions, &currentPerms)
				if currentPerms == nil {
					currentPerms = make(map[string]interface{})
				}

				if body.Permissions != nil {
					for k, v := range *body.Permissions {
						if k == "librariesAccessible" {
							continue
						}
						currentPerms[k] = v
					}
				}

				if len(body.LibrariesAccessible) > 0 {
					currentPerms["librariesAccessible"] = body.LibrariesAccessible
				}
				if body.ItemTagsSelected != nil {
					currentPerms["itemTagsSelected"] = *body.ItemTagsSelected
				}

				newPermsBytes, _ := json.Marshal(currentPerms)
				targetUser.Permissions = newPermsBytes
				hasUpdates = true
			}

			if hasUpdates {
				if shouldUpdateToken {
					secret := getTokenSecret(db)
					apiToken := jwt.NewWithClaims(jwt.SigningMethodHS256, &core.AuthClaims{
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

				nowStr := idb.TimeToDBStr(time.Now())
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
					log.Errorf("[idb.User Update] DB Error: %v", err)
					http.Error(w, `{"error": "Failed to update user"}`, http.StatusInternalServerError)
					return
				}
			}

			updatedUser, _ := idb.GetUserFullByID(r.Context(), db, targetUserID)
			userJSON := updatedUser.ToOldJSONForBrowser(userSess.Type != "root")

			if isocket.GlobalAuth != nil {
				isocket.GlobalAuth.BroadcastToUser(targetUserID, "user_updated", userJSON)
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"user":    userJSON,
			})
			return
		}

		if r.Method == http.MethodDelete {
			log.Infof("[Go] DELETE /api/users/%s", targetUserID)
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

			targetUser, err := idb.GetUserFullByID(r.Context(), db, targetUserID)
			if err != nil || targetUser == nil {
				http.Error(w, `{"error": "idb.User not found"}`, http.StatusNotFound)
				return
			}

			userJSON := targetUser.ToOldJSONForBrowser(userSess.Type != "root")

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

			if isocket.GlobalAuth != nil {
				isocket.GlobalAuth.BroadcastToAdmins("user_removed", userJSON)
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

type UserSessionResponse struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	IPAddress string `json:"ipAddress"`
	UserAgent string `json:"userAgent"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
	IsCurrent bool   `json:"isCurrent"`
}

func mapUserSessionToResponse(r *http.Request, s idb.UserSessionDB) UserSessionResponse {
	isCurrent := false
	cookie, err := r.Cookie("refresh_token")
	if err == nil && cookie.Value != "" {
		if s.RefreshToken == cookie.Value || s.LastRefreshToken == cookie.Value {
			isCurrent = true
		}
	}
	return UserSessionResponse{
		ID:        s.ID,
		UserID:    s.UserID,
		IPAddress: s.IPAddress,
		UserAgent: s.UserAgent,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
		IsCurrent: isCurrent,
	}
}

// handleGetUserLoginSessions returns active login sessions for a user
func handleGetUserLoginSessions(db *sql.DB, targetUserID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" && userSess.ID != targetUserID {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		sessions, err := idb.GetUserSessions(r.Context(), db, targetUserID)
		if err != nil {
			log.Errorf("[Login Sessions] Failed to get sessions: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		resp := make([]UserSessionResponse, len(sessions))
		for i, s := range sessions {
			resp[i] = mapUserSessionToResponse(r, s)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// handleDeleteUserLoginSession deletes/revokes a specific login session
func handleDeleteUserLoginSession(db *sql.DB, targetUserID string, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" && userSess.ID != targetUserID {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		// Verify session exists and belongs to this user
		var sessionUser string
		var sessionRefreshToken string
		err := db.QueryRowContext(r.Context(), "SELECT userId, refreshToken FROM sessions WHERE id = ?", sessionID).Scan(&sessionUser, &sessionRefreshToken)
		if err == sql.ErrNoRows {
			http.Error(w, `{"error": "Session not found"}`, http.StatusNotFound)
			return
		} else if err != nil {
			log.Errorf("[Delete Session] DB error: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		if sessionUser != targetUserID {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		// Delete session
		err = idb.DeleteSessionByID(r.Context(), db, sessionID)
		if err != nil {
			log.Errorf("[Delete Session] Failed to delete session: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		// If they deleted their own current session, clear the cookie
		cookie, err := r.Cookie("refresh_token")
		if err == nil && cookie.Value != "" && cookie.Value == sessionRefreshToken {
			http.SetCookie(w, &http.Cookie{
				Name:     "refresh_token",
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	}
}
