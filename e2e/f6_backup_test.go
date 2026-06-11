package e2e

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

func TestF6DatabaseBackups(t *testing.T) {
	// Set UNDER_TEST environment variable so syscall.Exec is skipped when applying backup
	os.Setenv("UNDER_TEST", "true")

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

	// Login (admin)
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

	// 2. Setup Normal User (Non-Admin)
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

	// Login as normal user
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

	var createdBackupID string
	var backupBytes []byte

	// --- Tier 1 Tests ---

	// 1. Create backup (POST /api/backups)
	t.Run("POST /api/backups - success", func(t *testing.T) {
		req, _ := http.NewRequest("POST", h.BaseURL+"/api/backups", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var respData map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		backupsList, ok := respData["backups"].([]interface{})
		if !ok || len(backupsList) == 0 {
			t.Fatalf("No backups returned or invalid structure: %v", respData)
		}

		backupInfo := backupsList[0].(map[string]interface{})
		createdBackupID = backupInfo["id"].(string)
		if createdBackupID == "" {
			t.Errorf("Backup ID is empty")
		}
	})

	// 2. List backups (GET /api/backups)
	t.Run("GET /api/backups - success", func(t *testing.T) {
		req, _ := http.NewRequest("GET", h.BaseURL+"/api/backups", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var respData map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		backupsList, ok := respData["backups"].([]interface{})
		if !ok || len(backupsList) == 0 {
			t.Fatalf("No backups returned or invalid structure: %v", respData)
		}

		found := false
		for _, b := range backupsList {
			backupInfo := b.(map[string]interface{})
			if backupInfo["id"].(string) == createdBackupID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Created backup ID %s not found in backups list", createdBackupID)
		}
	})

	// 3. Download backup (GET /api/backups/:id/download)
	t.Run("GET /api/backups/:id/download - success", func(t *testing.T) {
		req, _ := http.NewRequest("GET", h.BaseURL+"/api/backups/"+createdBackupID+"/download", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		backupBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		if len(backupBytes) == 0 {
			t.Errorf("Downloaded backup file is empty")
		}

		contentDisposition := resp.Header.Get("Content-Disposition")
		if !bytes.Contains([]byte(contentDisposition), []byte(createdBackupID+".audiobookshelf")) {
			t.Errorf("Expected Content-Disposition to contain filename, got: %s", contentDisposition)
		}
	})

	// 4. Delete backup (DELETE /api/backups/:id)
	t.Run("DELETE /api/backups/:id - success", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", h.BaseURL+"/api/backups/"+createdBackupID, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("DELETE failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200 for delete, got %d", resp.StatusCode)
		}

		// List again and verify it is gone
		reqList, _ := http.NewRequest("GET", h.BaseURL+"/api/backups", nil)
		reqList.Header.Set("Authorization", "Bearer "+adminToken)
		respList, err := client.Do(reqList)
		if err != nil {
			t.Fatalf("GET backups list failed: %v", err)
		}
		defer respList.Body.Close()

		var respData map[string]interface{}
		json.NewDecoder(respList.Body).Decode(&respData)
		if respData["backups"] != nil {
			backupsList := respData["backups"].([]interface{})
			for _, b := range backupsList {
				backupInfo := b.(map[string]interface{})
				if backupInfo["id"].(string) == createdBackupID {
					t.Errorf("Backup ID %s was still found in list after deletion", createdBackupID)
				}
			}
		}
	})

	// 5. Update backup path (POST /api/backups/path)
	t.Run("POST /api/backups/path - success", func(t *testing.T) {
		newPath := filepath.Join(h.ConfigDir, "new_backups_dir")
		payload := map[string]string{
			"path": newPath,
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", h.BaseURL+"/api/backups/path", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		// Verify path was created on disk
		if _, err := os.Stat(newPath); err != nil {
			t.Errorf("Expected new backup directory to be created, but got: %v", err)
		}

		// Restore path back to default path so other tests and ApplyBackup can find files in default
		defaultPath := filepath.Join(h.MetadataDir, "backups")
		payloadRestore := map[string]string{
			"path": defaultPath,
		}
		bodyRestore, _ := json.Marshal(payloadRestore)
		reqRestore, _ := http.NewRequest("POST", h.BaseURL+"/api/backups/path", bytes.NewReader(bodyRestore))
		reqRestore.Header.Set("Authorization", "Bearer "+adminToken)
		reqRestore.Header.Set("Content-Type", "application/json")
		respRestore, err := client.Do(reqRestore)
		if err != nil {
			t.Fatalf("Failed to restore backup path: %v", err)
		}
		respRestore.Body.Close()
		if respRestore.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200 for restore path, got %d", respRestore.StatusCode)
		}
	})

	// --- Tier 2 Tests ---

	// 6. Access control check (non-admin/normal user blocked from all backups endpoints with HTTP 403)
	t.Run("Access control - normal user blocked", func(t *testing.T) {
		endpoints := []struct {
			method string
			url    string
			body   io.Reader
		}{
			{"GET", h.BaseURL + "/api/backups", nil},
			{"POST", h.BaseURL + "/api/backups", nil},
			{"POST", h.BaseURL + "/api/backups/path", bytes.NewReader([]byte(`{"path":"/tmp"}`))},
			{"POST", h.BaseURL + "/api/backups/upload", nil},
			{"DELETE", h.BaseURL + "/api/backups/" + createdBackupID, nil},
			{"GET", h.BaseURL + "/api/backups/" + createdBackupID + "/download", nil},
			{"POST", h.BaseURL + "/api/backups/" + createdBackupID + "/apply", nil},
		}

		for _, ep := range endpoints {
			req, _ := http.NewRequest(ep.method, ep.url, ep.body)
			req.Header.Set("Authorization", "Bearer "+normalToken)
			if ep.body != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Errorf("Request to %s %s failed: %v", ep.method, ep.url, err)
				continue
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("Expected 403 Forbidden for %s %s, got %d", ep.method, ep.url, resp.StatusCode)
			}
		}
	})

	// 7. Reject invalid backup ID containing path traversal characters like ".." or "/" (HTTP 400 Bad Request)
	t.Run("Path traversal rejection", func(t *testing.T) {
		invalidIDs := []string{
			"some..id",
			"some\\\\id",
		}

		for _, badID := range invalidIDs {
			// Test DELETE
			reqDel, _ := http.NewRequest("DELETE", h.BaseURL+"/api/backups/"+badID, nil)
			reqDel.Header.Set("Authorization", "Bearer "+adminToken)
			respDel, err := client.Do(reqDel)
			if err != nil {
				t.Fatalf("Delete failed: %v", err)
			}
			respDel.Body.Close()
			if respDel.StatusCode != http.StatusBadRequest {
				t.Errorf("Expected status 400 for DELETE with invalid ID %q, got %d", badID, respDel.StatusCode)
			}

			// Test GET download
			reqDownload, _ := http.NewRequest("GET", h.BaseURL+"/api/backups/"+badID+"/download", nil)
			reqDownload.Header.Set("Authorization", "Bearer "+adminToken)
			respDownload, err := client.Do(reqDownload)
			if err != nil {
				t.Fatalf("Download failed: %v", err)
			}
			respDownload.Body.Close()
			if respDownload.StatusCode != http.StatusBadRequest {
				t.Errorf("Expected status 400 for GET download with invalid ID %q, got %d", badID, respDownload.StatusCode)
			}

			// Test POST apply
			reqApply, _ := http.NewRequest("POST", h.BaseURL+"/api/backups/"+badID+"/apply", nil)
			reqApply.Header.Set("Authorization", "Bearer "+adminToken)
			respApply, err := client.Do(reqApply)
			if err != nil {
				t.Fatalf("Apply failed: %v", err)
			}
			respApply.Body.Close()
			if respApply.StatusCode != http.StatusBadRequest {
				t.Errorf("Expected status 400 for POST apply with invalid ID %q, got %d", badID, respApply.StatusCode)
			}
		}
	})

	// 8. Return error/404 for downloading or deleting non-existent backup ID
	t.Run("Non-existent backup handling", func(t *testing.T) {
		nonExistentID := "nonexistent_backup_id_123"

		// DELETE
		reqDel, _ := http.NewRequest("DELETE", h.BaseURL+"/api/backups/"+nonExistentID, nil)
		reqDel.Header.Set("Authorization", "Bearer "+adminToken)
		respDel, err := client.Do(reqDel)
		if err != nil {
			t.Fatalf("DELETE failed: %v", err)
		}
		respDel.Body.Close()
		if respDel.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404 for DELETE non-existent backup, got %d", respDel.StatusCode)
		}

		// GET download
		reqDownload, _ := http.NewRequest("GET", h.BaseURL+"/api/backups/"+nonExistentID+"/download", nil)
		reqDownload.Header.Set("Authorization", "Bearer "+adminToken)
		respDownload, err := client.Do(reqDownload)
		if err != nil {
			t.Fatalf("GET download failed: %v", err)
		}
		respDownload.Body.Close()
		if respDownload.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404 for GET download non-existent backup, got %d", respDownload.StatusCode)
		}
	})

	// 9. Upload backup (POST /api/backups/upload)
	t.Run("POST /api/backups/upload - invalid suffix", func(t *testing.T) {
		bodyBuf := &bytes.Buffer{}
		bodyWriter := multipart.NewWriter(bodyBuf)
		fileWriter, err := bodyWriter.CreateFormFile("file", "fake_backup.zip")
		if err != nil {
			t.Fatalf("Failed to create form file: %v", err)
		}
		fileWriter.Write([]byte("some invalid zip content"))
		bodyWriter.Close()

		req, _ := http.NewRequest("POST", h.BaseURL+"/api/backups/upload", bodyBuf)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", bodyWriter.FormDataContentType())
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Upload failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400 for invalid suffix upload, got %d", resp.StatusCode)
		}
	})

	var uploadedBackupID string
	t.Run("POST /api/backups/upload - success with .audiobookshelf suffix", func(t *testing.T) {
		if len(backupBytes) == 0 {
			t.Fatalf("No backup bytes saved to upload")
		}

		bodyBuf := &bytes.Buffer{}
		bodyWriter := multipart.NewWriter(bodyBuf)
		uploadedBackupID = "uploaded-backup-test"
		fileWriter, err := bodyWriter.CreateFormFile("file", uploadedBackupID+".audiobookshelf")
		if err != nil {
			t.Fatalf("Failed to create form file: %v", err)
		}
		fileWriter.Write(backupBytes)
		bodyWriter.Close()

		reqUpload, _ := http.NewRequest("POST", h.BaseURL+"/api/backups/upload", bodyBuf)
		reqUpload.Header.Set("Authorization", "Bearer "+adminToken)
		reqUpload.Header.Set("Content-Type", bodyWriter.FormDataContentType())
		respUpload, err := client.Do(reqUpload)
		if err != nil {
			t.Fatalf("Upload failed: %v", err)
		}
		defer respUpload.Body.Close()

		if respUpload.StatusCode != http.StatusOK {
			respBytes, _ := io.ReadAll(respUpload.Body)
			t.Fatalf("Expected status 200 for upload, got %d. Body: %s", respUpload.StatusCode, string(respBytes))
		}

		var respData map[string]interface{}
		if err := json.NewDecoder(respUpload.Body).Decode(&respData); err != nil {
			t.Fatalf("Failed to decode upload response: %v", err)
		}

		backupsList, ok := respData["backups"].([]interface{})
		if !ok || len(backupsList) == 0 {
			t.Fatalf("Upload response missing backups list or empty: %v", respData)
		}

		found := false
		for _, b := range backupsList {
			backupInfo := b.(map[string]interface{})
			if backupInfo["filename"].(string) == uploadedBackupID+".audiobookshelf" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Uploaded backup filename %s.audiobookshelf not found in list", uploadedBackupID)
		}
	})

	// 10. Apply backup (POST /api/backups/:id/apply)
	t.Run("POST /api/backups/:id/apply - success", func(t *testing.T) {
		req, _ := http.NewRequest("POST", h.BaseURL+"/api/backups/"+uploadedBackupID+"/apply", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Apply request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBytes, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status 200 for apply, got %d. Body: %s", resp.StatusCode, string(respBytes))
		}
	})
}
