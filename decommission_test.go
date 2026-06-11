package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"

	"audiobookshelf/internal/core")

func prepareTestDB(t *testing.T) *sql.DB {
	db := setupTestDB(t)

	// Drop inadequate tables created by setupTestDB
	_, _ = db.Exec(`DROP TABLE IF EXISTS users`)
	_, _ = db.Exec(`DROP TABLE IF EXISTS settings`)

	// Recreate settings table
	_, err := db.Exec(`CREATE TABLE settings (
		key TEXT PRIMARY KEY,
		value TEXT,
		createdAt TEXT,
		updatedAt TEXT
	)`)
	if err != nil {
		t.Fatalf("Failed to create settings table: %v", err)
	}

	// Recreate users table
	_, err = db.Exec(`CREATE TABLE users (
		id TEXT PRIMARY KEY,
		username TEXT,
		email TEXT,
		pash TEXT,
		type TEXT,
		token TEXT,
		isActive INTEGER,
		isLocked INTEGER,
		lastSeen INTEGER,
		permissions TEXT,
		bookmarks TEXT,
		extraData TEXT,
		createdAt TEXT,
		updatedAt TEXT
	)`)
	if err != nil {
		t.Fatalf("Failed to create users table: %v", err)
	}

	// Insert default server settings
	_, err = db.Exec(`INSERT INTO settings (key, value, createdAt, updatedAt) VALUES ('server-settings', '{"sortingIgnorePrefix": true}', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert settings: %v", err)
	}

	// Create sessions table
	_, err = db.Exec(`CREATE TABLE sessions (
		id TEXT PRIMARY KEY,
		userId TEXT,
		ipAddress TEXT,
		userAgent TEXT,
		refreshToken TEXT,
		expiresAt TEXT,
		lastRefreshToken TEXT,
		lastRefreshTokenExpiresAt TEXT,
		createdAt TEXT,
		updatedAt TEXT
	)`)
	if err != nil {
		t.Fatalf("Failed to create sessions table: %v", err)
	}

	return db
}

func TestInitEndpoint(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	// Initial check: no root user exists, hasRootUser should be false
	hasRoot, err := HasRootUser(db)
	if err != nil {
		t.Fatalf("HasRootUser check failed: %v", err)
	}
	if hasRoot {
		t.Fatalf("Root user should not exist in clean test DB")
	}

	tests := []struct {
		name           string
		payload        string
		expectedStatus int
		checkDB        func(t *testing.T, db *sql.DB)
	}{
		{
			name:           "ValidRootInit",
			payload:        `{"newRoot": {"username": "admin-root", "password": "rootpassword"}}`,
			expectedStatus: http.StatusOK,
			checkDB: func(t *testing.T, db *sql.DB) {
				hasRoot, err = HasRootUser(db)
				if err != nil || !hasRoot {
					t.Errorf("Expected root user to be created, hasRoot = %t, err = %v", hasRoot, err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/init", bytes.NewBufferString(tt.payload))
			rr := httptest.NewRecorder()

			handler := handleInit(db)
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, rr.Code, rr.Body.String())
			}

			if tt.checkDB != nil {
				tt.checkDB(t, db)
			}
		})
	}
}

func TestLoginAndLogout(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	// Initialize the root user
	reqBody := `{"newRoot": {"username": "testroot", "password": "testpassword"}}`
	req := httptest.NewRequest("POST", "/init", bytes.NewBufferString(reqBody))
	rr := httptest.NewRecorder()
	handleInit(db).ServeHTTP(rr, req)

	var refreshCookie *http.Cookie

	t.Run("Login", func(t *testing.T) {
		loginBody := `{"username": "testroot", "password": "testpassword"}`
		req := httptest.NewRequest("POST", "/login", bytes.NewBufferString(loginBody))
		rr := httptest.NewRecorder()
		handleLogin(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected login success (200), got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse login response: %v", err)
		}

		userObj, ok := resp["user"].(map[string]interface{})
		if !ok || userObj["username"] != "testroot" {
			t.Errorf("Unexpected user object in login response: %v", resp)
		}

		// Verify cookie was set
		cookies := rr.Result().Cookies()
		for _, c := range cookies {
			if c.Name == "refresh_token" {
				refreshCookie = c
				break
			}
		}
		if refreshCookie == nil || refreshCookie.Value == "" {
			t.Errorf("Expected refresh_token cookie to be set")
		}
	})

	t.Run("Logout", func(t *testing.T) {
		logoutReq := httptest.NewRequest("POST", "/logout", nil)
		if refreshCookie != nil {
			logoutReq.AddCookie(refreshCookie)
		}
		rr := httptest.NewRecorder()
		handleLogout(db).ServeHTTP(rr, logoutReq)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected logout success (200), got %d", rr.Code)
		}
	})
}

func TestUsersCRUD(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	// Insert an admin user session context for authentication checks
	adminSession := &core.UserSession{
		ID:                 "admin-id",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}

	var createdUserID string

	t.Run("GetUsers", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/users", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handleGetUsers(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("CreateUser", func(t *testing.T) {
		createUserBody := `{"username": "newuser", "password": "newpassword", "type": "user", "isActive": true}`
		req := httptest.NewRequest("POST", "/api/users", bytes.NewBufferString(createUserBody))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handleUserCRUD(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var createResp map[string]interface{}
		json.Unmarshal(rr.Body.Bytes(), &createResp)
		createdUser := createResp["user"].(map[string]interface{})
		createdUserID = createdUser["id"].(string)

		if createdUser["username"] != "newuser" {
			t.Errorf("Expected username newuser, got %s", createdUser["username"])
		}
	})

	t.Run("UpdateUser", func(t *testing.T) {
		if createdUserID == "" {
			t.Skip("skipping update test, user not created")
		}
		updateUserBody := `{"email": "newuser@example.com"}`
		req := httptest.NewRequest("PATCH", "/api/users/"+createdUserID, bytes.NewBufferString(updateUserBody))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handleUserCRUD(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		// Verify change in DB
		user, err := getUserByID(context.Background(), db, createdUserID)
		if err != nil || user == nil || user.Email == nil || *user.Email != "newuser@example.com" {
			t.Errorf("Expected user email to be updated, got: %v", user)
		}
	})

	t.Run("DeleteUser", func(t *testing.T) {
		if createdUserID == "" {
			t.Skip("skipping delete test, user not created")
		}
		req := httptest.NewRequest("DELETE", "/api/users/"+createdUserID, nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handleUserCRUD(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200 for delete user, got %d", rr.Code)
		}

		// Verify deleted from DB
		deletedUser, _ := getUserByID(context.Background(), db, createdUserID)
		if deletedUser != nil {
			t.Errorf("Expected user to be deleted from DB")
		}
	})
}

func TestServerSettingsCRUD(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	adminSession := &core.UserSession{
		ID:       "admin-id",
		Username: "admin",
		Type:     "admin",
		IsActive: true,
	}

	t.Run("GetSettings", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/settings", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handleGetServerSettings(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", rr.Code)
		}
	})

	t.Run("UpdateSettings", func(t *testing.T) {
		updateSettingsBody := `{"language": "fr"}`
		req := httptest.NewRequest("POST", "/api/settings", bytes.NewBufferString(updateSettingsBody))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handleUpdateServerSettings(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		// Verify changed
		settings, err := GetServerSettings(db)
		if err != nil || settings.Language != "fr" {
			t.Errorf("Expected language to be updated to fr, got: %s", settings.Language)
		}
	})
}

func TestAuthorize(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	// Initialize the root user
	reqBody := `{"newRoot": {"username": "testroot", "password": "testpassword"}}`
	req := httptest.NewRequest("POST", "/init", bytes.NewBufferString(reqBody))
	rr := httptest.NewRecorder()
	handleInit(db).ServeHTTP(rr, req)

	user, err := getUserByUsername(context.Background(), db, "testroot")
	if err != nil || user == nil {
		t.Fatalf("Failed to fetch root user: %v", err)
	}

	userSession := &core.UserSession{
		ID:                 user.ID,
		Username:           user.Username,
		Type:               user.Type,
		IsActive:           user.IsActive,
		AccessAllLibraries: true,
	}

	tests := []struct {
		name           string
		session        *core.UserSession
		expectedStatus int
		checkResponse  func(t *testing.T, body string)
	}{
		{
			name:           "AuthorizedRoot",
			session:        userSession,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body string) {
				var resp map[string]interface{}
				if err := json.Unmarshal([]byte(body), &resp); err != nil {
					t.Fatalf("Failed to parse authorize response: %v", err)
				}
				userObj, ok := resp["user"].(map[string]interface{})
				if !ok || userObj["username"] != "testroot" {
					t.Errorf("Unexpected user object in authorize response: %v", resp)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/authorize", nil)
			if tt.session != nil {
				req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, tt.session))
			}
			rr := httptest.NewRecorder()
			handleAuthorize(db).ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, rr.Code, rr.Body.String())
			}

			if tt.checkResponse != nil {
				tt.checkResponse(t, rr.Body.String())
			}
		})
	}
}
