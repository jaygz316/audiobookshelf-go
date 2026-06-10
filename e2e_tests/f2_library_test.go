package e2e_tests

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

func TestF2Library(t *testing.T) {
	h := NewTestHarness()
	if err := h.Start(); err != nil {
		t.Fatalf("Failed to start harness: %v", err)
	}
	defer h.Stop()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("Failed to create cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}

	// 1. Setup Admin Root
	// POST /init
	initPayload := map[string]interface{}{
		"newRoot": map[string]string{
			"username": "rootadmin",
			"password": "supersecurepassword123",
		},
	}
	initBody, _ := json.Marshal(initPayload)
	resp, err := client.Post(h.BaseURL+"/init", "application/json", bytes.NewReader(initBody))
	if err != nil {
		t.Fatalf("Failed to initialize root: %v", err)
	}
	resp.Body.Close()

	// POST /login (admin)
	loginPayload := map[string]string{
		"username": "rootadmin",
		"password": "supersecurepassword123",
	}
	loginBody, _ := json.Marshal(loginPayload)
	resp, err = client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatalf("Failed to login admin: %v", err)
	}
	var adminResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&adminResp)
	resp.Body.Close()
	adminToken := adminResp["user"].(map[string]interface{})["accessToken"].(string)

	// 2. Insert normal user directly into DB (since POST /api/users is broken with RouterBasePath prefix)
	hashedPash, err := bcrypt.GenerateFromPassword([]byte("normalpassword123"), 8)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}
	dbUser, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer dbUser.Close()

	permsJSON := `{"download":true,"accessExplicitContent":false,"accessAllLibraries":true,"librariesAccessible":[],"accessAllTags":true,"itemTagsSelected":[],"selectedTagsNotAccessible":false}`
	_, err = dbUser.Exec(`INSERT INTO users (id, username, email, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, ?, ?, 1, ?, '{}', '[]', datetime('now'), datetime('now'))`,
		uuid.New().String(), "normaluser", "normal@test.com", "user", string(hashedPash), "token-normal", permsJSON)
	if err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}

	// Log in as normal user to get their token
	normalLoginPayload := map[string]string{
		"username": "normaluser",
		"password": "normalpassword123",
	}
	normalLoginBody, _ := json.Marshal(normalLoginPayload)
	resp, err = client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(normalLoginBody))
	if err != nil {
		t.Fatalf("Failed to login normal user: %v", err)
	}
	var normalResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&normalResp)
	resp.Body.Close()
	normalToken := normalResp["user"].(map[string]interface{})["accessToken"].(string)

	// 3. Test: Non-admin write operations are blocked (Tier 2)
	t.Run("POST /api/libraries - Forbidden for non-admin", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":      "Forbidden Library",
			"mediaType": "book",
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+normalToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("Expected status 403, got %d", resp.StatusCode)
		}
	})

	// 4. Test: Reject library creation with missing name (Tier 2)
	t.Run("POST /api/libraries - Missing name", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":      "",
			"mediaType": "book",
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", resp.StatusCode)
		}
	})

	// 5. Test: Reject folder paths pointing to invalid disk locations (Tier 2)
	t.Run("POST /api/libraries - Invalid folder path", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":      "Invalid Path Library",
			"mediaType": "book",
			"folders": []map[string]string{
				{"path": "/invalid\x00path"},
			},
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", resp.StatusCode)
		}
	})

	// 6. Test: Return 404 for invalid library ID (Tier 2)
	t.Run("GET /api/libraries/:id - NotFound", func(t *testing.T) {
		req, _ := http.NewRequest("GET", h.BaseURL+"/api/libraries/nonexistent-id", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", resp.StatusCode)
		}
	})

	// 7. Test: Create Library Success (Tier 1)
	var libraryID string
	var folderPath string
	t.Run("POST /api/libraries - Success", func(t *testing.T) {
		folderPath = filepath.Join(h.ConfigDir, "libraryA")
		payload := map[string]interface{}{
			"name":      "Library A",
			"mediaType": "book",
			"folders": []map[string]string{
				{"path": folderPath},
			},
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var createdLib map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&createdLib); err != nil {
			t.Fatalf("Failed to decode created library: %v", err)
		}

		libraryID = createdLib["id"].(string)
		if libraryID == "" {
			t.Errorf("Returned library ID is empty")
		}
		if createdLib["name"] != "Library A" {
			t.Errorf("Expected library name 'Library A', got %v", createdLib["name"])
		}

		// Verify folder was created on disk
		if _, err := os.Stat(folderPath); err != nil {
			t.Errorf("Expected folder to be created on disk, got error: %v", err)
		}
	})

	// 8. Test: List libraries (Tier 1)
	t.Run("GET /api/libraries - List & Verify", func(t *testing.T) {
		req, _ := http.NewRequest("GET", h.BaseURL+"/api/libraries", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var libsResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&libsResp); err != nil {
			t.Fatalf("Failed to decode libraries list: %v", err)
		}

		results := libsResp["libraries"].([]interface{})
		found := false
		for _, item := range results {
			lib := item.(map[string]interface{})
			if lib["id"] == libraryID {
				found = true
				if lib["name"] != "Library A" {
					t.Errorf("Expected library name 'Library A', got %v", lib["name"])
				}
				folders := lib["folders"].([]interface{})
				if len(folders) != 1 {
					t.Errorf("Expected 1 folder, got %d", len(folders))
				}
			}
		}
		if !found {
			t.Errorf("Library A not found in listed libraries")
		}
	})

	// 9. Test: Update library (Add a Folder) (Tier 1)
	var newFolderPath string
	t.Run("PATCH /api/libraries/:id - Add folder / Update settings", func(t *testing.T) {
		newFolderPath = filepath.Join(h.ConfigDir, "libraryA_updated")
		nameUpdate := "Library A Updated"
		payload := map[string]interface{}{
			"name": &nameUpdate,
			"folders": []map[string]interface{}{
				// Retain old folder by including path, but since no ID is passed
				// it acts as a re-creation/add. Actually, to add folders:
				// payload.Folders must contain both folders.
				{"path": folderPath},
				{"path": newFolderPath},
			},
			"settings": map[string]interface{}{
				"audiobooksOnly": true,
			},
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("PATCH", h.BaseURL+"/api/libraries/"+libraryID, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var updatedLib map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&updatedLib)
		if updatedLib["name"] != "Library A Updated" {
			t.Errorf("Library name update failed, got %v", updatedLib["name"])
		}

		folders := updatedLib["folders"].([]interface{})
		if len(folders) != 2 {
			t.Errorf("Expected 2 folders, got %d", len(folders))
		}

		// Check folder created on disk
		if _, err := os.Stat(newFolderPath); err != nil {
			t.Errorf("Expected updated folder to be created on disk: %v", err)
		}
	})

	// 10. Test: Delete Library & Cascade (Tier 1 & Tier 2)
	t.Run("DELETE /api/libraries/:id - Cascade deletion", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", h.BaseURL+"/api/libraries/"+libraryID, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		// Connect to DB directly to verify cascade deletion
		db, err := sql.Open("sqlite", h.DBPath)
		if err != nil {
			t.Fatalf("Failed to open DB: %v", err)
		}
		defer db.Close()

		var libCount int
		err = db.QueryRow("SELECT count(*) FROM libraries WHERE id = ?", libraryID).Scan(&libCount)
		if err != nil || libCount != 0 {
			t.Errorf("Library record was not deleted: %v, count=%d", err, libCount)
		}

		var folderCount int
		err = db.QueryRow("SELECT count(*) FROM libraryFolders WHERE libraryId = ?", libraryID).Scan(&folderCount)
		if err != nil || folderCount != 0 {
			t.Errorf("Library folders were not cascade-deleted: %v, count=%d", err, folderCount)
		}
	})
}
