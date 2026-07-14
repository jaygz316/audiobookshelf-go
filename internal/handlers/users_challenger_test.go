package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"audiobookshelf/internal/core"
)

func TestAdminDeletesRootUser_SecurityControl(t *testing.T) {
	db := setupUsersTestDB(t)
	defer db.Close()

	// 1. Insert a root user (which has a UUID)
	rootUserID := "root-uuid-1234"
	_, err := db.Exec(`
		INSERT INTO users (id, username, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt)
		VALUES (?, 'rootuser', 'root', 'somehash', 'sometoken', 1, '{}', '{}', '[]', '2026-06-19T14:00:00Z', '2026-06-19T14:00:00Z')
	`, rootUserID)
	if err != nil {
		t.Fatalf("failed to insert root user: %v", err)
	}

	// 2. Set up admin user session context
	adminSession := &core.UserSession{
		ID:                 "admin-id",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}

	// 3. Make DELETE request using the root user's UUID
	req := httptest.NewRequest("DELETE", "/api/users/"+rootUserID, nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
	rr := httptest.NewRecorder()

	// Call the handler
	handleUserCRUD(db).ServeHTTP(rr, req)

	// Check if delete was blocked.
	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden when admin attempts to delete root user, got %d", rr.Code)
	}

	// Verify root is still in the database
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM users WHERE id = ?", rootUserID).Scan(&count)
	if err != nil {
		t.Fatalf("db query failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected root user to still exist in DB, count: %d", count)
	}
}

func TestRootUserDisablesSelf_SecurityControl(t *testing.T) {
	db := setupUsersTestDB(t)
	defer db.Close()

	// 1. Insert a root user
	rootUserID := "root-uuid-5678"
	_, err := db.Exec(`
		INSERT INTO users (id, username, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt)
		VALUES (?, 'rootuser', 'root', 'somehash', 'sometoken', 1, '{}', '{}', '[]', '2026-06-19T14:00:00Z', '2026-06-19T14:00:00Z')
	`, rootUserID)
	if err != nil {
		t.Fatalf("failed to insert root user: %v", err)
	}

	// 2. Set up root user session context (root modifying itself)
	rootSession := &core.UserSession{
		ID:                 rootUserID,
		Username:           "rootuser",
		Type:               "root",
		IsActive:           true,
		AccessAllLibraries: true,
	}

	// 3. Make PATCH request to disable active status
	patchBody := `{"isActive": false}`
	req := httptest.NewRequest("PATCH", "/api/users/"+rootUserID, bytes.NewBufferString(patchBody))
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, rootSession))
	rr := httptest.NewRecorder()

	// Call the handler
	handleUserCRUD(db).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request when root user attempts to disable self, got %d", rr.Code)
	}

	// Verify root is still active in the database
	var isActive int
	err = db.QueryRow("SELECT isActive FROM users WHERE id = ?", rootUserID).Scan(&isActive)
	if err != nil {
		t.Fatalf("db query failed: %v", err)
	}
	if isActive != 1 {
		t.Errorf("Expected root user to remain active in DB, got: %d", isActive)
	}
}

func TestAdminEscalatesToRoot_SecurityControl(t *testing.T) {
	db := setupUsersTestDB(t)
	defer db.Close()

	// 1. Insert an admin user and a regular user
	adminUserID := "admin-uuid-1234"
	regularUserID := "user-uuid-5678"
	_, err := db.Exec(`
		INSERT INTO users (id, username, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt)
		VALUES (?, 'adminuser', 'admin', 'somehash', 'sometoken', 1, '{}', '{}', '[]', '2026-06-19T14:00:00Z', '2026-06-19T14:00:00Z')
	`, adminUserID)
	if err != nil {
		t.Fatalf("failed to insert admin user: %v", err)
	}
	_, err = db.Exec(`
		INSERT INTO users (id, username, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt)
		VALUES (?, 'regularuser', 'user', 'somehash', 'sometoken', 1, '{}', '{}', '[]', '2026-06-19T14:00:00Z', '2026-06-19T14:00:00Z')
	`, regularUserID)
	if err != nil {
		t.Fatalf("failed to insert regular user: %v", err)
	}

	// 2. Set up admin user session context (admin modifying regular user)
	adminSession := &core.UserSession{
		ID:                 adminUserID,
		Username:           "adminuser",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}

	// 3. Make PATCH request to escalate regular user to root
	patchBody := `{"type": "root"}`
	req := httptest.NewRequest("PATCH", "/api/users/"+regularUserID, bytes.NewBufferString(patchBody))
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
	rr := httptest.NewRecorder()

	// Call the handler
	handleUserCRUD(db).ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden when admin attempts to escalate user to root, got %d", rr.Code)
	}

	// Verify user type in the database
	var userType string
	err = db.QueryRow("SELECT type FROM users WHERE id = ?", regularUserID).Scan(&userType)
	if err != nil {
		t.Fatalf("db query failed: %v", err)
	}
	if userType != "user" {
		t.Errorf("Expected user type to remain 'user', got %s", userType)
	}
}

func TestUserFormValidationConstraints(t *testing.T) {
	db := setupUsersTestDB(t)
	defer db.Close()

	adminSession := &core.UserSession{
		ID:                 "admin-id",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}

	// 1. POST without username
	t.Run("CreateWithoutUsername", func(t *testing.T) {
		body := `{"password": "mypassword", "type": "user", "isActive": true}`
		req := httptest.NewRequest("POST", "/api/users", bytes.NewBufferString(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handleUserCRUD(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected 400 Bad Request, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	// 2. POST without password
	t.Run("CreateWithoutPassword", func(t *testing.T) {
		body := `{"username": "newuser", "type": "user", "isActive": true}`
		req := httptest.NewRequest("POST", "/api/users", bytes.NewBufferString(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handleUserCRUD(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected 400 Bad Request, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	// 3. POST valid user
	var createdUserID string
	t.Run("CreateValidUser", func(t *testing.T) {
		body := `{"username": "validuser", "password": "validpassword", "type": "user", "isActive": true}`
		req := httptest.NewRequest("POST", "/api/users", bytes.NewBufferString(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handleUserCRUD(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]interface{}
		json.Unmarshal(rr.Body.Bytes(), &resp)
		u := resp["user"].(map[string]interface{})
		createdUserID = u["id"].(string)
	})

	// 4. PATCH with empty password (should keep current, succeed)
	t.Run("EditWithEmptyPassword", func(t *testing.T) {
		if createdUserID == "" {
			t.Skip("user not created")
		}
		body := `{"password": "", "email": "valid@example.com"}`
		req := httptest.NewRequest("PATCH", "/api/users/"+createdUserID, bytes.NewBufferString(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handleUserCRUD(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	// 5. POST root user as admin (should be rejected)
	t.Run("CreateRootUserAsAdmin", func(t *testing.T) {
		body := `{"username": "attemptedroot", "password": "password", "type": "root", "isActive": true}`
		req := httptest.NewRequest("POST", "/api/users", bytes.NewBufferString(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handleUserCRUD(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden when admin attempts to create root user, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})
}
