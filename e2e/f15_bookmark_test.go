package e2e

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"testing"

	_ "modernc.org/sqlite"
)

func TestF15Bookmarks(t *testing.T) {
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

	// 1. Setup Admin Root & login
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

	// 2. Open DB to insert a dummy library and item
	db, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('lib-123', 'My Library', 'book')`)
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}
	_, err = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title) VALUES ('item-123', 'lib-123', 'book', 'book-123', 'Test Book')`)
	if err != nil {
		t.Fatalf("Failed to insert libraryItem: %v", err)
	}

	// 3. Create a Bookmark
	t.Run("Create Bookmark", func(t *testing.T) {
		createPayload := map[string]interface{}{
			"time":  123.45,
			"title": "First Mark",
		}
		createBody, _ := json.Marshal(createPayload)
		req, _ := http.NewRequest("POST", h.BaseURL+"/api/me/item/item-123/bookmark", bytes.NewReader(createBody))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("POST bookmark failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var created map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&created)
		if created["title"] != "First Mark" {
			t.Errorf("Expected title 'First Mark', got %q", created["title"])
		}
		if created["time"] != 123.45 {
			t.Errorf("Expected time 123.45, got %v", created["time"])
		}
	})

	// 4. Verify Bookmark retrieved via GET /api/me
	t.Run("Get /api/me contains bookmark", func(t *testing.T) {
		req, _ := http.NewRequest("GET", h.BaseURL+"/api/me", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("GET /api/me failed: %v", err)
		}
		defer resp.Body.Close()

		var meResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&meResp)
		bookmarksList := meResp["bookmarks"].([]interface{})
		if len(bookmarksList) != 1 {
			t.Fatalf("Expected 1 bookmark, got %d", len(bookmarksList))
		}

		b := bookmarksList[0].(map[string]interface{})
		if b["title"] != "First Mark" {
			t.Errorf("Expected bookmark title 'First Mark', got %q", b["title"])
		}
	})

	// 5. Update Bookmark
	t.Run("Update Bookmark", func(t *testing.T) {
		updatePayload := map[string]interface{}{
			"time":  123.45,
			"title": "Updated Mark Title",
		}
		updateBody, _ := json.Marshal(updatePayload)
		req, _ := http.NewRequest("PATCH", h.BaseURL+"/api/me/item/item-123/bookmark", bytes.NewReader(updateBody))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("PATCH bookmark failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var updated map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&updated)
		if updated["title"] != "Updated Mark Title" {
			t.Errorf("Expected updated title 'Updated Mark Title', got %q", updated["title"])
		}
	})

	// 6. Delete Bookmark
	t.Run("Delete Bookmark", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", h.BaseURL+"/api/me/item/item-123/bookmark/123.45", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("DELETE bookmark failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		// Verify zero bookmarks in database now
		var bookmarksBytes []byte
		err = db.QueryRow("SELECT bookmarks FROM users WHERE username = 'rootadmin'").Scan(&bookmarksBytes)
		if err != nil {
			t.Fatalf("Query bookmarks from user failed: %v", err)
		}
		var bookmarksList []interface{}
		if len(bookmarksBytes) > 0 {
			json.Unmarshal(bookmarksBytes, &bookmarksList)
		}
		if len(bookmarksList) != 0 {
			t.Errorf("Expected 0 bookmarks in user record, got %d", len(bookmarksList))
		}
	})
}
