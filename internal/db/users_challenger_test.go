package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUsersChallenger(t *testing.T) {
	// Setup test database
	tmpDir, err := os.MkdirTemp("", "testdb-users-challenger")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()

	// 1. Initial State Check
	hasRoot, err := HasRootUser(database)
	if err != nil {
		t.Fatalf("HasRootUser failed: %v", err)
	}
	if hasRoot {
		t.Errorf("Expected no root user in fresh DB")
	}

	// 2. Insert User manually
	userId := "user-challenger-1"
	username := "challenger"
	email := "challenger@example.com"
	pash := "$2a$10$NotRealHash..."
	userType := "user"
	token := "token-challenger"
	perms := GetDefaultPermissionsForUserType(userType)
	nowStr := TimeToDBStr(time.Now())

	_, err = database.ExecContext(ctx, `
		INSERT INTO users (id, username, email, pash, type, token, isActive, isLocked, permissions, bookmarks, extraData, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, ?, ?, 1, 0, ?, '[]', '{"authOpenIDSub": "sub-1"}', ?, ?)`,
		userId, username, email, pash, userType, token, perms, nowStr, nowStr)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	// Verify CheckUserExistsWithUsername
	exists, err := CheckUserExistsWithUsername(ctx, database, username)
	if err != nil {
		t.Fatalf("CheckUserExistsWithUsername failed: %v", err)
	}
	if !exists {
		t.Errorf("Expected user %s to exist", username)
	}

	exists, err = CheckUserExistsWithUsername(ctx, database, "nonexistent")
	if err != nil {
		t.Fatalf("CheckUserExistsWithUsername failed for nonexistent: %v", err)
	}
	if exists {
		t.Errorf("Expected nonexistent user not to exist")
	}

	// Test GetUserFullByID
	u, err := GetUserFullByID(ctx, database, userId)
	if err != nil {
		t.Fatalf("GetUserFullByID failed: %v", err)
	}
	if u == nil {
		t.Fatalf("Expected to find user by ID %s", userId)
	}
	if u.Username != username {
		t.Errorf("Expected username %s, got %s", username, u.Username)
	}
	if u.Email == nil || *u.Email != email {
		t.Errorf("Expected email %s, got %v", email, u.Email)
	}
	if u.Type != userType {
		t.Errorf("Expected type %s, got %s", userType, u.Type)
	}

	// Test GetUserFullByUsername
	uByUsername, err := GetUserFullByUsername(ctx, database, username)
	if err != nil {
		t.Fatalf("GetUserFullByUsername failed: %v", err)
	}
	if uByUsername == nil {
		t.Fatalf("Expected to find user by username %s", username)
	}
	if uByUsername.ID != userId {
		t.Errorf("Expected ID %s, got %s", userId, uByUsername.ID)
	}

	// Test ToOldJSONForBrowser
	oldJSON := u.ToOldJSONForBrowser(false)
	if oldJSON["id"] != userId {
		t.Errorf("ToOldJSONForBrowser id mismatch: %v", oldJSON["id"])
	}
	if oldJSON["username"] != username {
		t.Errorf("ToOldJSONForBrowser username mismatch: %v", oldJSON["username"])
	}

	// Test GetUserByID (auth_users.go)
	userSess, err := GetUserByID(database, userId)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if userSess == nil {
		t.Fatalf("Expected UserSession to not be nil")
	}
	if userSess.Username != username {
		t.Errorf("Expected UserSession username to be %s, got %s", username, userSess.Username)
	}

	// Test GetUserByIDOrOldID
	// Case 1: Fetch by normal ID
	userSess2, err := GetUserByIDOrOldID(database, userId)
	if err != nil {
		t.Fatalf("GetUserByIDOrOldID failed: %v", err)
	}
	if userSess2 == nil {
		t.Fatalf("Expected UserSession to not be nil")
	}
	if userSess2.ID != userId {
		t.Errorf("Expected ID %s, got %s", userId, userSess2.ID)
	}

	// Case 2: Fetch by oldUserId
	userId2 := "user-challenger-2"
	username2 := "challenger2"
	_, err = database.ExecContext(ctx, `
		INSERT INTO users (id, username, email, pash, type, token, isActive, isLocked, permissions, bookmarks, extraData, createdAt, updatedAt)
		VALUES (?, ?, NULL, NULL, 'user', 'token2', 1, 0, ?, '[]', '{"oldUserId": "old-id-2"}', ?, ?)`,
		userId2, username2, perms, nowStr, nowStr)
	if err != nil {
		t.Fatalf("Failed to insert second test user: %v", err)
	}

	userSess3, err := GetUserByIDOrOldID(database, "old-id-2")
	if err != nil {
		t.Fatalf("GetUserByIDOrOldID with old ID failed: %v", err)
	}
	if userSess3 == nil {
		t.Fatalf("Expected UserSession with old ID to not be nil")
	}
	if userSess3.ID != userId2 {
		t.Errorf("Expected ID %s, got %s for old ID", userId2, userSess3.ID)
	}

	// Case 3: Fetch user with NULL extraData
	userId3 := "user-challenger-3"
	username3 := "challenger3"
	_, err = database.ExecContext(ctx, `
		INSERT INTO users (id, username, email, pash, type, token, isActive, isLocked, permissions, bookmarks, extraData, createdAt, updatedAt)
		VALUES (?, ?, NULL, NULL, 'user', 'token3', 1, 0, ?, '[]', NULL, ?, ?)`,
		userId3, username3, perms, nowStr, nowStr)
	if err != nil {
		t.Fatalf("Failed to insert third test user: %v", err)
	}

	userSess4, err := GetUserByIDOrOldID(database, userId3)
	if err != nil {
		t.Fatalf("GetUserByIDOrOldID with NULL extraData failed: %v", err)
	}
	if userSess4 == nil {
		t.Fatalf("Expected UserSession with NULL extraData to not be nil")
	}
	if userSess4.ID != userId3 {
		t.Errorf("Expected ID %s, got %s for user with NULL extraData", userId3, userSess4.ID)
	}

	// 3. User Sessions CRUD Testing
	refreshToken := "refresh-token-challenger"
	expiresAt := time.Now().Add(24 * time.Hour)
	err = CreateSession(ctx, database, userId, "127.0.0.1", "Go-Test-Agent", refreshToken, expiresAt)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	sessions, err := GetUserSessions(ctx, database, userId)
	if err != nil {
		t.Fatalf("GetUserSessions failed: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(sessions))
	} else {
		if sessions[0].RefreshToken != refreshToken {
			t.Errorf("Expected refresh token %s, got %s", refreshToken, sessions[0].RefreshToken)
		}
		if sessions[0].IPAddress != "127.0.0.1" {
			t.Errorf("Expected IP 127.0.0.1, got %s", sessions[0].IPAddress)
		}
	}

	// Test deleting session by refresh token
	affected, err := DeleteSessionByRefreshToken(ctx, database, refreshToken)
	if err != nil {
		t.Fatalf("DeleteSessionByRefreshToken failed: %v", err)
	}
	if affected != 1 {
		t.Errorf("Expected 1 row affected by delete, got %d", affected)
	}

	// Verify session is deleted
	sessions, err = GetUserSessions(ctx, database, userId)
	if err != nil {
		t.Fatalf("GetUserSessions failed after delete: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("Expected 0 sessions, got %d", len(sessions))
	}

	// Create an expired session to test CleanupExpiredSessions
	err = CreateSession(ctx, database, userId, "127.0.0.1", "Go-Test-Agent-Expired", "expired-token", time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("CreateSession for expired failed: %v", err)
	}

	cleaned, err := CleanupExpiredSessions(ctx, database)
	if err != nil {
		t.Fatalf("CleanupExpiredSessions failed: %v", err)
	}
	if cleaned != 1 {
		t.Errorf("Expected 1 cleaned session, got %d", cleaned)
	}

	// Create another session to test DeleteSessionByID
	err = CreateSession(ctx, database, userId, "127.0.0.1", "Go-Test-Agent", "session-id-token", expiresAt)
	if err != nil {
		t.Fatalf("CreateSession for delete by ID failed: %v", err)
	}
	sessions, err = GetUserSessions(ctx, database, userId)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("Failed to create session for delete by ID: %v", err)
	}

	err = DeleteSessionByID(ctx, database, sessions[0].ID)
	if err != nil {
		t.Fatalf("DeleteSessionByID failed: %v", err)
	}

	// Verify deleted
	sessions, err = GetUserSessions(ctx, database, userId)
	if err != nil || len(sessions) != 0 {
		t.Errorf("Expected session to be deleted by ID")
	}

	// 4. API Key Verification
	// Insert test API key
	keyID := "test-api-key-id"
	_, err = database.ExecContext(ctx, "INSERT INTO apiKeys (id, isActive, expiresAt, userId, name, createdAt) VALUES (?, 1, ?, ?, 'test-key', ?)",
		keyID, TimeToDBStr(time.Now().Add(time.Hour)), userId, nowStr)
	if err != nil {
		t.Fatalf("Failed to insert API key: %v", err)
	}

	apiKeySess, err := CheckAPIKey(database, keyID)
	if err != nil {
		t.Fatalf("CheckAPIKey failed: %v", err)
	}
	if apiKeySess == nil {
		t.Fatalf("Expected API Key UserSession not to be nil")
	}
	if apiKeySess.ID != userId {
		t.Errorf("Expected API Key session user ID %s, got %s", userId, apiKeySess.ID)
	}

	// 5. OIDC Operations Testing
	userinfo := map[string]interface{}{
		"sub":                "sub-oidc-123",
		"preferred_username": "oidc_user",
		"email":              "oidc@example.com",
		"email_verified":     true,
	}

	// Test FindUserFromOpenIdUserInfo before user is created
	foundUser, err := FindUserFromOpenIdUserInfo(ctx, database, userinfo, "username")
	if err != nil {
		t.Fatalf("FindUserFromOpenIdUserInfo failed on empty: %v", err)
	}
	if foundUser != nil {
		t.Errorf("Expected no user to be found initially")
	}

	// Test CreateUserFromOpenIdUserInfo
	tokenSecret := "secret"
	createdUser, err := CreateUserFromOpenIdUserInfo(ctx, database, userinfo, tokenSecret, "user")
	if err != nil {
		t.Fatalf("CreateUserFromOpenIdUserInfo failed: %v", err)
	}
	if createdUser == nil {
		t.Fatalf("Expected created user to not be nil")
	}
	if createdUser.Username != "oidc_user" {
		t.Errorf("Expected created user username 'oidc_user', got %s", createdUser.Username)
	}

	// Test FindUserFromOpenIdUserInfo after user is created (by sub)
	foundUserBySub, err := FindUserFromOpenIdUserInfo(ctx, database, userinfo, "username")
	if err != nil {
		t.Fatalf("FindUserFromOpenIdUserInfo failed: %v", err)
	}
	if foundUserBySub == nil {
		t.Fatalf("Expected user to be found by sub")
	}
	if foundUserBySub.ID != createdUser.ID {
		t.Errorf("Found user ID mismatch: expected %s, got %s", createdUser.ID, foundUserBySub.ID)
	}

	// Test FindUserFromOpenIdUserInfo match by email
	// First let's remove the sub link from the user to test matching by email
	_, err = database.ExecContext(ctx, "UPDATE users SET extraData = '{}' WHERE id = ?", createdUser.ID)
	if err != nil {
		t.Fatalf("Failed to reset extraData: %v", err)
	}

	foundUserByEmail, err := FindUserFromOpenIdUserInfo(ctx, database, userinfo, "email")
	if err != nil {
		t.Fatalf("FindUserFromOpenIdUserInfo (email) failed: %v", err)
	}
	if foundUserByEmail == nil {
		t.Fatalf("Expected user to be found by email matching")
	}

	// Test UpdateUserTypeAndToken
	err = UpdateUserTypeAndToken(ctx, database, foundUserByEmail, "admin", tokenSecret)
	if err != nil {
		t.Fatalf("UpdateUserTypeAndToken failed: %v", err)
	}
	if foundUserByEmail.Type != "admin" {
		t.Errorf("Expected updated type to be admin, got %s", foundUserByEmail.Type)
	}

	// 6. GetUserLoginPayload Verification
	payload, err := GetUserLoginPayload(ctx, database, foundUserByEmail)
	if err != nil {
		t.Fatalf("GetUserLoginPayload failed: %v", err)
	}
	if payload == nil {
		t.Fatalf("Expected non-nil login payload")
	}
	// Verify that if type is admin, "users" field is included in the payload
	usersList, exists := payload["users"]
	if !exists {
		t.Errorf("Expected 'users' list to exist in admin login payload")
	} else {
		lst, ok := usersList.([]map[string]interface{})
		if !ok {
			t.Errorf("Expected 'users' to be a list of maps, got %T", usersList)
		}
		if len(lst) < 3 { // challenger, challenger2, oidc_user
			t.Errorf("Expected at least 3 users in list, got %d", len(lst))
		}
	}
}
