package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

func TestLibraryCRUD_AccessControls(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// 1. Create temporary directories for folders
	tempDir := t.TempDir()
	folderPath := filepath.Join(tempDir, "audiobooks")
	if err := os.MkdirAll(folderPath, 0755); err != nil {
		t.Fatalf("Failed to create temp subfolder: %v", err)
	}

	payload := idb.CreateLibraryPayload{
		Name:      "Test Lib",
		MediaType: "book",
		Icon:      "database",
		Provider:  "google",
		Folders: []idb.CreateFolderPayload{
			{Path: folderPath, FullPath: folderPath},
		},
	}
	payloadBytes, _ := json.Marshal(payload)

	// --- A. HandleCreateLibrary ---
	t.Run("CreateLibrary_Unauthorized", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/libraries", bytes.NewReader(payloadBytes))
		rr := httptest.NewRecorder()
		HandleCreateLibrary(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401 Unauthorized, got %d", rr.Code)
		}
	})

	t.Run("CreateLibrary_ForbiddenForNormalUser", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/libraries", bytes.NewReader(payloadBytes))
		sess := &core.UserSession{
			ID:       "user-normal",
			Username: "normal",
			Type:     "user",
			IsActive: true,
		}
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, sess))
		rr := httptest.NewRecorder()
		HandleCreateLibrary(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden, got %d", rr.Code)
		}
	})

	t.Run("CreateLibrary_AdminSuccess", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/libraries", bytes.NewReader(payloadBytes))
		sess := &core.UserSession{
			ID:       "user-admin",
			Username: "admin",
			Type:     "admin",
			IsActive: true,
		}
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, sess))
		rr := httptest.NewRecorder()
		HandleCreateLibrary(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var createdLib idb.LibraryJSON
		if err := json.NewDecoder(rr.Body).Decode(&createdLib); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		if createdLib.Name != "Test Lib" {
			t.Errorf("Expected library name 'Test Lib', got %s", createdLib.Name)
		}
	})

	// Add a dummy library for remaining tests
	adminSess := &core.UserSession{
		ID:       "user-admin",
		Username: "admin",
		Type:     "admin",
		IsActive: true,
	}
	dummyLib, err := idb.CreateLibrary(db, &payload)
	if err != nil {
		t.Fatalf("Failed to create dummy library in db: %v", err)
	}

	// --- B. HandleGetLibraries ---
	t.Run("GetLibraries_Unauthorized", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/libraries", nil)
		rr := httptest.NewRecorder()
		HandleGetLibraries(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401 Unauthorized, got %d", rr.Code)
		}
	})

	t.Run("GetLibraries_NormalUser_NoAccess", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/libraries", nil)
		// User has no libraries listed in their libraries access (empty slice or nil)
		sess := &core.UserSession{
			ID:                  "user-normal",
			Username:            "normal",
			Type:                "user",
			IsActive:            true,
			AccessAllLibraries:  false,
			LibrariesAccessible: []string{}, // No access
		}
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, sess))
		rr := httptest.NewRecorder()
		HandleGetLibraries(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d", rr.Code)
		}
		var resp map[string][]*idb.LibraryJSON
		json.Unmarshal(rr.Body.Bytes(), &resp)
		if len(resp["libraries"]) != 0 {
			t.Errorf("Expected 0 libraries, got %d", len(resp["libraries"]))
		}
	})

	t.Run("GetLibraries_NormalUser_WithAccess", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/libraries", nil)
		sess := &core.UserSession{
			ID:                  "user-normal",
			Username:            "normal",
			Type:                "user",
			IsActive:            true,
			AccessAllLibraries:  false,
			LibrariesAccessible: []string{dummyLib.ID}, // Access to dummyLib
		}
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, sess))
		rr := httptest.NewRecorder()
		HandleGetLibraries(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d", rr.Code)
		}
		var resp map[string][]*idb.LibraryJSON
		json.Unmarshal(rr.Body.Bytes(), &resp)
		if len(resp["libraries"]) != 1 {
			t.Errorf("Expected 1 library, got %d", len(resp["libraries"]))
		}
	})

	// --- C. HandleGetLibraryByID ---
	t.Run("GetLibraryByID_Unauthorized", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/libraries/"+dummyLib.ID, nil)
		rr := httptest.NewRecorder()
		HandleGetLibraryByID(db, dummyLib.ID).ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401 Unauthorized, got %d", rr.Code)
		}
	})

	t.Run("GetLibraryByID_NormalUser_NoAccess", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/libraries/"+dummyLib.ID, nil)
		sess := &core.UserSession{
			ID:                  "user-normal",
			Username:            "normal",
			Type:                "user",
			IsActive:            true,
			AccessAllLibraries:  false,
			LibrariesAccessible: []string{}, // No access
		}
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, sess))
		rr := httptest.NewRecorder()
		HandleGetLibraryByID(db, dummyLib.ID).ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden, got %d", rr.Code)
		}
	})

	t.Run("GetLibraryByID_NormalUser_WithAccess", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/libraries/"+dummyLib.ID, nil)
		sess := &core.UserSession{
			ID:                  "user-normal",
			Username:            "normal",
			Type:                "user",
			IsActive:            true,
			AccessAllLibraries:  false,
			LibrariesAccessible: []string{dummyLib.ID},
		}
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, sess))
		rr := httptest.NewRecorder()
		HandleGetLibraryByID(db, dummyLib.ID).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", rr.Code)
		}
	})

	t.Run("GetLibraryByID_NonExistent", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/libraries/nonexistent", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSess))
		rr := httptest.NewRecorder()
		HandleGetLibraryByID(db, "nonexistent").ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected 404 Not Found, got %d", rr.Code)
		}
	})

	// --- D. HandleUpdateLibrary ---
	updateName := "Updated Lib Name"
	updatePayload := idb.UpdateLibraryPayload{
		Name: &updateName,
	}
	updatePayloadBytes, _ := json.Marshal(updatePayload)

	t.Run("UpdateLibrary_Unauthorized", func(t *testing.T) {
		req := httptest.NewRequest("PATCH", "/api/libraries/"+dummyLib.ID, bytes.NewReader(updatePayloadBytes))
		rr := httptest.NewRecorder()
		HandleUpdateLibrary(db, dummyLib.ID).ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401 Unauthorized, got %d", rr.Code)
		}
	})

	t.Run("UpdateLibrary_ForbiddenForNormalUser", func(t *testing.T) {
		req := httptest.NewRequest("PATCH", "/api/libraries/"+dummyLib.ID, bytes.NewReader(updatePayloadBytes))
		sess := &core.UserSession{
			ID:       "user-normal",
			Username: "normal",
			Type:     "user",
			IsActive: true,
		}
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, sess))
		rr := httptest.NewRecorder()
		HandleUpdateLibrary(db, dummyLib.ID).ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden, got %d", rr.Code)
		}
	})

	t.Run("UpdateLibrary_AdminSuccess", func(t *testing.T) {
		req := httptest.NewRequest("PATCH", "/api/libraries/"+dummyLib.ID, bytes.NewReader(updatePayloadBytes))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSess))
		rr := httptest.NewRecorder()
		HandleUpdateLibrary(db, dummyLib.ID).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		var updatedLib idb.LibraryJSON
		json.Unmarshal(rr.Body.Bytes(), &updatedLib)
		if updatedLib.Name != "Updated Lib Name" {
			t.Errorf("Expected updated name 'Updated Lib Name', got %s", updatedLib.Name)
		}
	})

	t.Run("UpdateLibrary_NonExistent", func(t *testing.T) {
		req := httptest.NewRequest("PATCH", "/api/libraries/nonexistent", bytes.NewReader(updatePayloadBytes))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSess))
		rr := httptest.NewRecorder()
		HandleUpdateLibrary(db, "nonexistent").ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected 404 Not Found, got %d", rr.Code)
		}
	})

	// --- E. HandleDeleteLibrary ---
	t.Run("DeleteLibrary_Unauthorized", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/libraries/"+dummyLib.ID, nil)
		rr := httptest.NewRecorder()
		HandleDeleteLibrary(db, dummyLib.ID).ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401 Unauthorized, got %d", rr.Code)
		}
	})

	t.Run("DeleteLibrary_ForbiddenForNormalUserWithoutDeletePermission", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/libraries/"+dummyLib.ID, nil)
		sess := &core.UserSession{
			ID:        "user-normal",
			Username:  "normal",
			Type:      "user",
			IsActive:  true,
			CanDelete: false,
		}
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, sess))
		rr := httptest.NewRecorder()
		HandleDeleteLibrary(db, dummyLib.ID).ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden, got %d", rr.Code)
		}
	})

	t.Run("DeleteLibrary_NormalUserWithDeletePermissionSuccess", func(t *testing.T) {
		// Normal user with CanDelete = true should be allowed
		req := httptest.NewRequest("DELETE", "/api/libraries/"+dummyLib.ID, nil)
		sess := &core.UserSession{
			ID:        "user-normal",
			Username:  "normal",
			Type:      "user",
			IsActive:  true,
			CanDelete: true,
		}
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, sess))
		rr := httptest.NewRecorder()
		HandleDeleteLibrary(db, dummyLib.ID).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		var deletedLib idb.LibraryJSON
		json.Unmarshal(rr.Body.Bytes(), &deletedLib)
		if deletedLib.ID != dummyLib.ID {
			t.Errorf("Expected deleted library ID %s, got %s", dummyLib.ID, deletedLib.ID)
		}

		// Verify it is actually deleted from DB
		libCheck, err := idb.GetLibraryByID(db, dummyLib.ID)
		if err != nil {
			t.Fatalf("DB error: %v", err)
		}
		if libCheck != nil {
			t.Errorf("Expected library to be nil after deletion, but got %+v", libCheck)
		}
	})

	t.Run("DeleteLibrary_NonExistent", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/libraries/nonexistent", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSess))
		rr := httptest.NewRecorder()
		HandleDeleteLibrary(db, "nonexistent").ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected 404 Not Found, got %d", rr.Code)
		}
	})
}

func TestLibraryCRUD_InputValidation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	adminSess := &core.UserSession{
		ID:       "user-admin",
		Username: "admin",
		Type:     "admin",
		IsActive: true,
	}

	t.Run("CreateLibrary_EmptyName", func(t *testing.T) {
		payload := idb.CreateLibraryPayload{
			Name:      "",
			MediaType: "book",
		}
		payloadBytes, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/libraries", bytes.NewReader(payloadBytes))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSess))
		rr := httptest.NewRecorder()
		HandleCreateLibrary(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected 400 Bad Request, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("CreateLibrary_InvalidJSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/libraries", bytes.NewReader([]byte("{invalid-json")))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSess))
		rr := httptest.NewRecorder()
		HandleCreateLibrary(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected 400 Bad Request, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("UpdateLibrary_InvalidJSON", func(t *testing.T) {
		req := httptest.NewRequest("PATCH", "/api/libraries/some-id", bytes.NewReader([]byte("{invalid-json")))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSess))
		rr := httptest.NewRecorder()
		HandleUpdateLibrary(db, "some-id").ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected 400 Bad Request, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})
}
