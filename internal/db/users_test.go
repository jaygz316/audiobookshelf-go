package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestDBForUsers(t *testing.T) (*sql.DB, string) {
	tmpDir, err := os.MkdirTemp("", "testdb-users-dir")
	if err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(tmpDir, "test_users.db")
	database, err := InitDB(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	return database, tmpDir
}

func TestToOldJSONForBrowser(t *testing.T) {
	emailStr := "test@example.com"
	lastSeen := int64(1672531199000)
	user := &User{
		ID:          "user_1",
		Username:    "testuser",
		Email:       &emailStr,
		Type:        "user",
		Token:       "secret_token_123",
		IsActive:    true,
		IsLocked:    false,
		LastSeen:    &lastSeen,
		Permissions: json.RawMessage(`{"download":true,"accessExplicitContent":false,"librariesAccessible":["lib_1"],"itemTagsSelected":["tag_1"]}`),
		Bookmarks:   json.RawMessage(`[{"id":"bm_1","time":10.5}]`),
		ExtraData:   json.RawMessage(`{"seriesHideFromContinueListening":["series_1"],"authOpenIDSub":"sub_123"}`),
		CreatedAt:   1672531190000,
		UpdatedAt:   1672531195000,
	}

	payload := user.ToOldJSONForBrowser(false)
	if payload["id"] != "user_1" {
		t.Errorf("Expected id user_1, got %v", payload["id"])
	}
	if payload["username"] != "testuser" {
		t.Errorf("Expected username testuser, got %v", payload["username"])
	}
	if payload["email"] == nil || *(payload["email"].(*string)) != "test@example.com" {
		t.Errorf("Expected email test@example.com, got %v", payload["email"])
	}
	if payload["token"] != "secret_token_123" {
		t.Errorf("Expected token secret_token_123, got %v", payload["token"])
	}
	if payload["hasOpenIDLink"] != true {
		t.Errorf("Expected hasOpenIDLink true, got %v", payload["hasOpenIDLink"])
	}

	// Test with root user hideRootToken = true
	rootUser := &User{
		ID:    "root_id",
		Type:  "root",
		Token: "root_token",
	}
	rootPayload := rootUser.ToOldJSONForBrowser(true)
	if rootPayload["token"] != "" {
		t.Errorf("Expected empty root token when hidden, got %v", rootPayload["token"])
	}
}

func TestUserQueries(t *testing.T) {
	db, tmpDir := setupTestDBForUsers(t)
	defer func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}()

	ctx := context.Background()

	// Initially non-existent
	u, err := GetUserFullByUsername(ctx, db, "missing")
	if err != nil {
		t.Fatalf("GetUserFullByUsername failed: %v", err)
	}
	if u != nil {
		t.Errorf("Expected nil user, got %+v", u)
	}

	u, err = GetUserFullByID(ctx, db, "missing")
	if err != nil {
		t.Fatalf("GetUserFullByID failed: %v", err)
	}
	if u != nil {
		t.Errorf("Expected nil user, got %+v", u)
	}

	exists, err := CheckUserExistsWithUsername(ctx, db, "missing")
	if err != nil {
		t.Fatalf("CheckUserExistsWithUsername failed: %v", err)
	}
	if exists {
		t.Errorf("Expected false for missing user")
	}

	// Create user manually
	createdAtStr := TimeToDBStr(time.Now())
	_, err = db.ExecContext(ctx, `INSERT INTO users (id, username, email, pash, type, token, isActive, isLocked, permissions, bookmarks, extraData, createdAt, updatedAt)
		VALUES ('u_1', 'user_1', 'user1@example.com', 'pash_123', 'admin', 'tok_123', 1, 0, '{}', '[]', '{}', ?, ?)`, createdAtStr, createdAtStr)
	if err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}

	// Fetch and verify by Username
	u, err = GetUserFullByUsername(ctx, db, "user_1")
	if err != nil {
		t.Fatalf("GetUserFullByUsername failed: %v", err)
	}
	if u == nil {
		t.Fatal("Expected user_1, got nil")
	}
	if u.ID != "u_1" || u.Username != "user_1" || u.Email == nil || *u.Email != "user1@example.com" || u.Type != "admin" || u.Token != "tok_123" {
		t.Errorf("Unexpected user fields: %+v", u)
	}

	// Fetch and verify by ID
	u, err = GetUserFullByID(ctx, db, "u_1")
	if err != nil {
		t.Fatalf("GetUserFullByID failed: %v", err)
	}
	if u == nil {
		t.Fatal("Expected user_1 by ID, got nil")
	}
	if u.Username != "user_1" {
		t.Errorf("Unexpected username: %s", u.Username)
	}

	// Check user exists
	exists, err = CheckUserExistsWithUsername(ctx, db, "user_1")
	if err != nil {
		t.Fatalf("CheckUserExistsWithUsername failed: %v", err)
	}
	if !exists {
		t.Errorf("Expected true for user_1")
	}
}

func TestUserSessions(t *testing.T) {
	db, tmpDir := setupTestDBForUsers(t)
	defer func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}()

	ctx := context.Background()
	userID := "user_s1"

	// Initial sessions empty
	sessList, err := GetUserSessions(ctx, db, userID)
	if err != nil {
		t.Fatalf("GetUserSessions failed: %v", err)
	}
	if len(sessList) != 0 {
		t.Errorf("Expected empty sessions, got %d", len(sessList))
	}

	// Create sessions
	expiresAt1 := time.Now().Add(24 * time.Hour)
	err = CreateSession(ctx, db, userID, "192.168.1.10", "Mozilla", "refresh_tok_1", expiresAt1)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	expiresAt2 := time.Now().Add(-1 * time.Hour) // expired session
	err = CreateSession(ctx, db, userID, "192.168.1.11", "Chrome", "refresh_tok_2", expiresAt2)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Retrieve sessions
	sessList, err = GetUserSessions(ctx, db, userID)
	if err != nil {
		t.Fatalf("GetUserSessions failed: %v", err)
	}
	if len(sessList) != 2 {
		t.Errorf("Expected 2 sessions, got %d", len(sessList))
	}

	// Match elements
	var activeSess, expiredSess UserSessionDB
	for _, s := range sessList {
		if s.RefreshToken == "refresh_tok_1" {
			activeSess = s
		} else if s.RefreshToken == "refresh_tok_2" {
			expiredSess = s
		}
	}
	if activeSess.IPAddress != "192.168.1.10" || activeSess.UserAgent != "Mozilla" {
		t.Errorf("Unexpected active session: %+v", activeSess)
	}
	if expiredSess.IPAddress != "192.168.1.11" || expiredSess.UserAgent != "Chrome" {
		t.Errorf("Unexpected expired session: %+v", expiredSess)
	}

	// Cleanup expired sessions
	deletedCount, err := CleanupExpiredSessions(ctx, db)
	if err != nil {
		t.Fatalf("CleanupExpiredSessions failed: %v", err)
	}
	if deletedCount != 1 {
		t.Errorf("Expected 1 deleted session, got %d", deletedCount)
	}

	sessList, err = GetUserSessions(ctx, db, userID)
	if err != nil {
		t.Fatalf("GetUserSessions failed: %v", err)
	}
	if len(sessList) != 1 {
		t.Errorf("Expected 1 session remaining, got %d", len(sessList))
	}
	if sessList[0].RefreshToken != "refresh_tok_1" {
		t.Errorf("Expected remaining session to be refresh_tok_1, got %s", sessList[0].RefreshToken)
	}

	// Delete session by ID
	err = DeleteSessionByID(ctx, db, sessList[0].ID)
	if err != nil {
		t.Fatalf("DeleteSessionByID failed: %v", err)
	}

	sessList, err = GetUserSessions(ctx, db, userID)
	if err != nil {
		t.Fatalf("GetUserSessions failed: %v", err)
	}
	if len(sessList) != 0 {
		t.Errorf("Expected 0 sessions after deletion, got %d", len(sessList))
	}

	// Test DeleteSessionByRefreshToken
	err = CreateSession(ctx, db, userID, "192.168.1.12", "Safari", "refresh_tok_3", expiresAt1)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	rowsAffected, err := DeleteSessionByRefreshToken(ctx, db, "refresh_tok_3")
	if err != nil {
		t.Fatalf("DeleteSessionByRefreshToken failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Errorf("Expected 1 row affected, got %d", rowsAffected)
	}
}

func TestUserOIDC(t *testing.T) {
	db, tmpDir := setupTestDBForUsers(t)
	defer func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}()

	ctx := context.Background()
	tokenSecret := "oidc_token_secret_value_1234567890"

	// Mock OIDC user info
	userInfo := map[string]interface{}{
		"sub":                "sub_oidc_999",
		"preferred_username": "oidc_user",
		"email":              "oidc@example.com",
		"email_verified":     true,
	}

	// Create user from OIDC
	user, err := CreateUserFromOpenIdUserInfo(ctx, db, userInfo, tokenSecret, "user")
	if err != nil {
		t.Fatalf("CreateUserFromOpenIdUserInfo failed: %v", err)
	}
	if user == nil {
		t.Fatal("Expected created user, got nil")
	}
	if user.Username != "oidc_user" || user.Email == nil || *user.Email != "oidc@example.com" || user.Type != "user" {
		t.Errorf("Unexpected user fields: %+v", user)
	}

	// Find OIDC user by sub
	u, err := FindUserFromOpenIdUserInfo(ctx, db, userInfo, "sub")
	if err != nil {
		t.Fatalf("FindUserFromOpenIdUserInfo failed: %v", err)
	}
	if u == nil || u.ID != user.ID {
		t.Errorf("Expected to find user by sub, got: %+v", u)
	}

	// Find by email linking
	// Create another user manually with same email but no OpenID sub
	_, err = db.ExecContext(ctx, `INSERT INTO users (id, username, email, pash, type, token, isActive, isLocked, permissions, bookmarks, extraData, createdAt, updatedAt)
		VALUES ('u_link_email', 'link_email', 'linkemail@example.com', 'pash', 'user', 'tok', 1, 0, '{}', '[]', '{}', '2026-07-16 12:00:00', '2026-07-16 12:00:00')`)
	if err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}

	userInfo2 := map[string]interface{}{
		"sub":            "sub_new_link",
		"email":          "linkemail@example.com",
		"email_verified": true,
	}
	u, err = FindUserFromOpenIdUserInfo(ctx, db, userInfo2, "email")
	if err != nil {
		t.Fatalf("FindUserFromOpenIdUserInfo by email failed: %v", err)
	}
	if u == nil || u.ID != "u_link_email" {
		t.Fatalf("Expected to link user by email, got: %+v", u)
	}
	// Verify extraData updated with authOpenIDSub
	var extra map[string]interface{}
	err = json.Unmarshal(u.ExtraData, &extra)
	if err != nil {
		t.Fatalf("Failed to unmarshal extraData: %v", err)
	}
	if extra["authOpenIDSub"] != "sub_new_link" {
		t.Errorf("Expected authOpenIDSub to be linked to sub_new_link, got %v", extra["authOpenIDSub"])
	}

	// UpdateUserTypeAndToken
	err = UpdateUserTypeAndToken(ctx, db, u, "admin", tokenSecret)
	if err != nil {
		t.Fatalf("UpdateUserTypeAndToken failed: %v", err)
	}
	if u.Type != "admin" {
		t.Errorf("Expected updated type to be admin, got %s", u.Type)
	}

	// Find by username linking
	_, err = db.ExecContext(ctx, `INSERT INTO users (id, username, email, pash, type, token, isActive, isLocked, permissions, bookmarks, extraData, createdAt, updatedAt)
		VALUES ('u_link_uname', 'link_uname_test', NULL, 'pash', 'user', 'tok', 1, 0, '{}', '[]', '{}', '2026-07-16 12:00:00', '2026-07-16 12:00:00')`)
	if err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}

	userInfo3 := map[string]interface{}{
		"sub":                "sub_new_link_uname",
		"preferred_username": "link_uname_test",
	}
	u, err = FindUserFromOpenIdUserInfo(ctx, db, userInfo3, "username")
	if err != nil {
		t.Fatalf("FindUserFromOpenIdUserInfo by username failed: %v", err)
	}
	if u == nil || u.ID != "u_link_uname" {
		t.Fatalf("Expected to link user by username, got: %+v", u)
	}
}

func TestGetUserLoginPayload(t *testing.T) {
	db, tmpDir := setupTestDBForUsers(t)
	defer func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}()

	ctx := context.Background()

	// Insert settings
	_, err := db.ExecContext(ctx, "UPDATE settings SET value = '{\"language\":\"fr-fr\",\"theme\":\"light\",\"tokenSecret\":\"secret\"}', updatedAt = 'now' WHERE key = 'server-settings'")
	if err != nil {
		t.Fatalf("Failed to update settings: %v", err)
	}

	// Insert a library
	_, err = db.ExecContext(ctx, "INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, lastScan, lastScanVersion, settings, createdAt, updatedAt) VALUES ('lib_payload_1', 'Main Lib', 1, 'icon', 'book', 'provider', NULL, NULL, '{}', 'now', 'now')")
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}

	// Insert admin and user
	_, err = db.ExecContext(ctx, `INSERT INTO users (id, username, email, pash, type, token, isActive, isLocked, permissions, bookmarks, extraData, createdAt, updatedAt)
		VALUES ('admin_id', 'admin_user', 'admin@example.com', 'pash', 'admin', 'token_admin', 1, 0, '{"accessAllLibraries":true}', '[]', '{}', 'now', 'now')`)
	if err != nil {
		t.Fatalf("Failed to insert admin: %v", err)
	}

	_, err = db.ExecContext(ctx, `INSERT INTO users (id, username, email, pash, type, token, isActive, isLocked, permissions, bookmarks, extraData, createdAt, updatedAt)
		VALUES ('user_id', 'regular_user', 'user@example.com', 'pash', 'user', 'token_user', 1, 0, '{"accessAllLibraries":true}', '[]', '{}', 'now', 'now')`)
	if err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}

	// 1. regular user payload (must not contain "users" field)
	regUser, err := GetUserFullByID(ctx, db, "user_id")
	if err != nil {
		t.Fatal(err)
	}
	regPayload, err := GetUserLoginPayload(ctx, db, regUser)
	if err != nil {
		t.Fatalf("GetUserLoginPayload for regular user failed: %v", err)
	}
	if regPayload["users"] != nil {
		t.Errorf("Regular user payload should not contain users, got %+v", regPayload["users"])
	}
	if regPayload["userDefaultLibraryId"] != "lib_payload_1" {
		t.Errorf("Expected userDefaultLibraryId lib_payload_1, got %v", regPayload["userDefaultLibraryId"])
	}

	// 2. admin user payload (must contain "users" field with all users)
	admUser, err := GetUserFullByID(ctx, db, "admin_id")
	if err != nil {
		t.Fatal(err)
	}
	admPayload, err := GetUserLoginPayload(ctx, db, admUser)
	if err != nil {
		t.Fatalf("GetUserLoginPayload for admin failed: %v", err)
	}
	usersList, ok := admPayload["users"].([]map[string]interface{})
	if !ok {
		t.Fatalf("Admin payload missing users slice of maps, got: %+v", admPayload["users"])
	}
	if len(usersList) != 2 {
		t.Errorf("Expected 2 users in admin payload, got %d", len(usersList))
	}
}

func TestParseTimeStr(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"", 0},
		{"1672531199000", 1672531199000},
		{"2023-01-01T00:00:00Z", 1672531200000},
		{"2023-01-01 00:00:00.000 +00:00", 1672531200000},
		{"2023-01-01 00:00:00", 1672531200000},
		{"invalid", 0},
	}

	for _, tc := range tests {
		got := ParseTimeStr(tc.input)
		if got != tc.expected {
			t.Errorf("ParseTimeStr(%q) = %d; expected %d", tc.input, got, tc.expected)
		}
	}
}

func TestGetDefaultPermissionsForUserType(t *testing.T) {
	userPerms := GetDefaultPermissionsForUserType("user")
	var m map[string]interface{}
	json.Unmarshal([]byte(userPerms), &m)
	if m["accessExplicitContent"] != false {
		t.Errorf("Expected accessExplicitContent to be false for user, got %v", m["accessExplicitContent"])
	}

	adminPerms := GetDefaultPermissionsForUserType("admin")
	json.Unmarshal([]byte(adminPerms), &m)
	if m["accessExplicitContent"] != true {
		t.Errorf("Expected accessExplicitContent to be true for admin, got %v", m["accessExplicitContent"])
	}
}
