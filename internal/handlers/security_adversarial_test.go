package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"audiobookshelf/internal/core"
	isocket "audiobookshelf/internal/socket"
)

// TestSecurity_AuthMiddlewareRejectRefresh verifies that AuthMiddleware rejects
// refresh tokens when they are presented as access tokens.
func TestSecurity_AuthMiddlewareRejectRefresh(t *testing.T) {
	db := setupSessionsTestDB(t)
	defer db.Close()

	// 1. Insert active user
	userID := "user-sec-1"
	_, err := db.Exec(`
		INSERT INTO users (id, username, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt)
		VALUES (?, 'secuser', 'user', 'hash', 'token', 1, '{}', '{}', '[]', 'now', 'now')
	`, userID)
	if err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}

	// 2. Insert active session (required by standard JWT validation)
	_, err = db.Exec(`
		INSERT INTO sessions (id, userId, ipAddress, userAgent, refreshToken, expiresAt, lastRefreshToken, lastRefreshTokenExpiresAt, createdAt, updatedAt)
		VALUES ('sess-sec-1', ?, '127.0.0.1', 'Agent', 'rt-sec-1', '2026-08-11T00:00:00Z', NULL, NULL, 'now', 'now')
	`, userID)
	if err != nil {
		t.Fatalf("Failed to insert session: %v", err)
	}

	secret := getTokenSecret(db)

	// A. Generate token of type "refresh"
	claimsRefresh := &core.AuthClaims{
		UserID:   userID,
		Username: "secuser",
		Type:     "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
	tokenRefresh, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claimsRefresh).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("Failed to sign refresh token: %v", err)
	}

	// B. Generate token of type "access"
	claimsAccess := &core.AuthClaims{
		UserID:   userID,
		Username: "secuser",
		Type:     "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
	tokenAccess, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claimsAccess).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("Failed to sign access token: %v", err)
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := AuthMiddleware(db, secret, nextHandler)

	// Test case 1: Refresh token as Authorization header -> should be rejected
	req1 := httptest.NewRequest("GET", "/api/me", nil)
	req1.Header.Set("Authorization", "Bearer "+tokenRefresh)
	rr1 := httptest.NewRecorder()
	mw.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized when using a refresh token as an access token, got %d", rr1.Code)
	}

	// Test case 2: Access token -> should succeed
	req2 := httptest.NewRequest("GET", "/api/me", nil)
	req2.Header.Set("Authorization", "Bearer "+tokenAccess)
	rr2 := httptest.NewRecorder()
	mw.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Errorf("Expected 200 OK when using a valid access token, got %d. Body: %s", rr2.Code, rr2.Body.String())
	}
}

// TestSecurity_SocketValidateTokenRejectRefresh verifies that socket's ValidateToken
// rejects tokens of type "refresh".
func TestSecurity_SocketValidateTokenRejectRefresh(t *testing.T) {
	secret := "my-secret-key-12345"

	// A. Refresh token
	claimsRefresh := &core.AuthClaims{
		UserID:   "user-1",
		Username: "secuser",
		Type:     "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
	tokenRefresh, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claimsRefresh).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("Failed to sign refresh token: %v", err)
	}

	// B. Access token
	claimsAccess := &core.AuthClaims{
		UserID:   "user-1",
		Username: "secuser",
		Type:     "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
	tokenAccess, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claimsAccess).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("Failed to sign access token: %v", err)
	}

	// Test refresh token -> should error
	userID, err := isocket.ValidateToken(tokenRefresh, secret)
	if err == nil {
		t.Errorf("Expected socket ValidateToken to error for refresh token, but it succeeded returning user %q", userID)
	}

	// Test access token -> should succeed
	userID, err = isocket.ValidateToken(tokenAccess, secret)
	if err != nil {
		t.Errorf("Expected socket ValidateToken to succeed for access token, but got error: %v", err)
	}
	if userID != "user-1" {
		t.Errorf("Expected user ID %q, got %q", "user-1", userID)
	}
}

// TestSecurity_MetricsAuthProtection verifies that the /metrics route requires admin or root permissions.
func TestSecurity_MetricsAuthProtection(t *testing.T) {
	db := setupSessionsTestDB(t)
	defer db.Close()

	// 1. Regular user session
	userSess := &core.UserSession{
		ID:       "user-reg",
		Username: "reguser",
		Type:     "user",
		IsActive: true,
	}

	// 2. Admin user session
	adminSess := &core.UserSession{
		ID:       "user-admin",
		Username: "adminuser",
		Type:     "admin",
		IsActive: true,
	}

	handler := handleMetrics(db)

	// A. Unauthorized (no context user session)
	req1 := httptest.NewRequest("GET", "/metrics", nil)
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized for /metrics with no authenticated user context, got %d", rr1.Code)
	}

	// B. Forbidden (regular user)
	req2 := httptest.NewRequest("GET", "/metrics", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), core.UserContextKey, userSess))
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden for /metrics with standard user context, got %d", rr2.Code)
	}

	// C. OK (admin user)
	req3 := httptest.NewRequest("GET", "/metrics", nil)
	req3 = req3.WithContext(context.WithValue(req3.Context(), core.UserContextKey, adminSess))
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for /metrics with admin user context, got %d", rr3.Code)
	}
}

// TestSecurity_MeUpdatePasswordInvalidatesSessionsAndToken verifies that when
// handleUpdateMePassword is run, it successfully updates the user's pash, regenerates
// their permanent token, and deletes all active sessions for that user.
func TestSecurity_MeUpdatePasswordInvalidatesSessionsAndToken(t *testing.T) {
	db := setupSessionsTestDB(t)
	defer db.Close()

	userID := "user-pwd-1"
	origHashed, _ := bcrypt.GenerateFromPassword([]byte("currentpass"), 8)
	origToken := "old-permanent-token"

	_, err := db.Exec(`
		INSERT INTO users (id, username, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt)
		VALUES (?, 'pwduser', 'user', ?, ?, 1, '{}', '{}', '[]', 'now', 'now')
	`, userID, string(origHashed), origToken)
	if err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}

	// Insert two active sessions
	_, err = db.Exec(`
		INSERT INTO sessions (id, userId, ipAddress, userAgent, refreshToken, expiresAt, lastRefreshToken, lastRefreshTokenExpiresAt, createdAt, updatedAt)
		VALUES 
		('sess-pwd-1', ?, '127.0.0.1', 'Browser', 'rt-1', 'expiry', NULL, NULL, 'now', 'now'),
		('sess-pwd-2', ?, '192.168.1.1', 'Mobile', 'rt-2', 'expiry', NULL, NULL, 'now', 'now')
	`, userID, userID)
	if err != nil {
		t.Fatalf("Failed to insert sessions: %v", err)
	}

	userSess := &core.UserSession{
		ID:       userID,
		Username: "pwduser",
		Type:     "user",
		IsActive: true,
	}

	// Construct payload
	body := map[string]string{
		"password":    "currentpass",
		"newPassword": "newsupersecretpass",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("PATCH", "/api/me/password", bytes.NewReader(bodyBytes))
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	// Execute handler
	handleUpdateMePassword(db).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from handleUpdateMePassword, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	// Verify database changes
	var hashedPash, token string
	err = db.QueryRow("SELECT pash, token FROM users WHERE id = ?", userID).Scan(&hashedPash, &token)
	if err != nil {
		t.Fatalf("Failed to query user after update: %v", err)
	}

	// Check password hash is updated and valid for new password
	err = bcrypt.CompareHashAndPassword([]byte(hashedPash), []byte("newsupersecretpass"))
	if err != nil {
		t.Errorf("New password hash is invalid: %v", err)
	}

	// Check permanent token is updated/changed from old one
	if token == origToken || token == "" {
		t.Errorf("Permanent token was not updated/regenerated. Expected changes, got %q", token)
	}

	// Verify that the new permanent token is a valid signed JWT containing the correct claims
	claims := &core.AuthClaims{}
	jwtToken, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(getTokenSecret(db)), nil
	})
	if err != nil || !jwtToken.Valid {
		t.Errorf("New permanent token is not a valid JWT: %v", err)
	} else {
		if claims.UserID != userID || claims.Username != "pwduser" || claims.Type != "user" {
			t.Errorf("New permanent token contains incorrect claims: %+v", claims)
		}
	}

	// Check sessions were cleared
	var sessionCount int
	err = db.QueryRow("SELECT COUNT(*) FROM sessions WHERE userId = ?", userID).Scan(&sessionCount)
	if err != nil {
		t.Fatalf("Failed to query sessions count: %v", err)
	}
	if sessionCount != 0 {
		t.Errorf("Expected all sessions for user to be deleted, but found %d remaining", sessionCount)
	}
}

// TestSecurity_LibraryAccessControls verifies that users cannot access resources
// (authors, series, narrators, playlists, collections, OPML, and items) in libraries
// they do not have access to, or items containing explicit content/restricted tags.
func TestSecurity_LibraryAccessControls(t *testing.T) {
	db := setupSessionsTestDB(t)
	defer db.Close()

	// Create libraryItems table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS libraryItems (
			id TEXT PRIMARY KEY, ino TEXT, libraryId TEXT, libraryFolderId TEXT,
			path TEXT, relPath TEXT, isFile INTEGER, mtime TEXT, ctime TEXT,
			birthtime TEXT, createdAt TEXT, updatedAt TEXT, isMissing INTEGER,
			isInvalid INTEGER, mediaType TEXT, mediaId TEXT, size INTEGER
		)
	`)
	if err != nil {
		t.Fatalf("failed to create libraryItems: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS books (
			id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT, subtitle TEXT,
			publishedYear TEXT, publishedDate TEXT, publisher TEXT, description TEXT,
			isbn TEXT, asin TEXT, language TEXT, explicit INTEGER, abridged INTEGER,
			coverPath TEXT, duration REAL, narrators BLOB, audioFiles BLOB,
			ebookFile BLOB, chapters BLOB, tags BLOB, genres BLOB, lockedFields BLOB
		)
	`)
	if err != nil {
		t.Fatalf("failed to create books: %v", err)
	}

	// Insert a library item belonging to "restricted-lib"
	_, err = db.Exec(`
		INSERT INTO libraryItems (id, ino, libraryId, libraryFolderId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size)
		VALUES ('item-1', 'ino-1', 'restricted-lib', 'folder-1', '/path/1', 'rel/1', 1, '2026-07-14T08:00:00Z', '2026-07-14T08:00:00Z', '2026-07-14T08:00:00Z', '2026-07-14T08:00:00Z', '2026-07-14T08:00:00Z', 0, 0, 'book', 'book-1', 100)
	`)
	if err != nil {
		t.Fatalf("failed to insert library item: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO books (id, title, titleIgnorePrefix, subtitle, publishedYear, publishedDate, publisher, description, isbn, asin, language, explicit, abridged, coverPath, duration, narrators, audioFiles, ebookFile, chapters, tags, genres, lockedFields)
		VALUES ('book-1', 'Secret Book', 'Secret Book', 'Sub', '2026', '2026', 'Pub', 'Desc', 'ISBN', 'ASIN', 'en', 1, 0, '/cover.jpg', 3600.0, '[]', '[]', '{}', '[]', '["secret-tag"]', '[]', '[]')
	`)
	if err != nil {
		t.Fatalf("failed to insert book: %v", err)
	}

	// Create user sessions:
	// 1. User with no access to "restricted-lib"
	noAccessUser := &core.UserSession{
		ID:                  "user-no-access",
		Username:            "noaccess",
		Type:                "user",
		IsActive:            true,
		AccessAllLibraries:  false,
		LibrariesAccessible: []string{"public-lib"},
	}

	// 2. User with access to "restricted-lib" but no explicit access
	noExplicitUser := &core.UserSession{
		ID:                       "user-no-explicit",
		Username:                 "noexplicit",
		Type:                     "user",
		IsActive:                 true,
		AccessAllLibraries:       false,
		LibrariesAccessible:      []string{"restricted-lib"},
		CanAccessExplicitContent: false,
		AccessAllTags:            true,
	}

	// 3. User with access to "restricted-lib" but restricted from "secret-tag"
	tagRestrictedUser := &core.UserSession{
		ID:                        "user-tag-restricted",
		Username:                  "tagrestricted",
		Type:                      "user",
		IsActive:                  true,
		AccessAllLibraries:        false,
		LibrariesAccessible:       []string{"restricted-lib"},
		CanAccessExplicitContent:  true,
		AccessAllTags:             false,
		ItemTagsSelected:          []string{"secret-tag"},
		SelectedTagsNotAccessible: true,
	}

	// Handlers to test for CanAccessLibrary checks:
	libraryHandlers := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"handleGetLibraryAuthors", handleGetLibraryAuthors(db, "restricted-lib")},
		{"handleGetLibrarySeries", handleGetLibrarySeries(db, "restricted-lib")},
		{"handleGetLibrarySeriesByID", handleGetLibrarySeriesByID(db, "restricted-lib", "series-1")},
		{"handleGetLibraryNarrators", handleGetLibraryNarrators(db, "restricted-lib")},
		{"handleGetLibraryPlaylists", handleGetLibraryPlaylists(db, "restricted-lib")},
		{"handleGetLibraryCollections", handleGetLibraryCollections(db, "restricted-lib")},
		{"handleGetLibraryOPML", handleGetLibraryOPML(db, "restricted-lib")},
	}

	for _, tc := range libraryHandlers {
		t.Run(tc.name+"_Forbidden", func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, noAccessUser))
			rr := httptest.NewRecorder()
			tc.handler.ServeHTTP(rr, req)
			if rr.Code != http.StatusForbidden {
				t.Errorf("expected 403 Forbidden for %s, got %d", tc.name, rr.Code)
			}
		})
	}

	// Test handleGetLibraryItemByID with different permission configurations:
	t.Run("handleGetLibraryItemByID_NoLibraryAccess", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/items/item-1", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, noAccessUser))
		rr := httptest.NewRecorder()
		handleGetLibraryItemByID(db, "item-1").ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for library access check, got %d", rr.Code)
		}
	})

	t.Run("handleGetLibraryItemByID_NoExplicitAccess", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/items/item-1", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, noExplicitUser))
		rr := httptest.NewRecorder()
		handleGetLibraryItemByID(db, "item-1").ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for explicit content check, got %d", rr.Code)
		}
	})

	t.Run("handleGetLibraryItemByID_TagRestricted", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/items/item-1", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, tagRestrictedUser))
		rr := httptest.NewRecorder()
		handleGetLibraryItemByID(db, "item-1").ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for tag restriction check, got %d", rr.Code)
		}
	})
}

