package e2e

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestF13BatchEdit(t *testing.T) {
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

	// Create Library
	libraryPath := filepath.Join(h.ConfigDir, "libraryBatch")
	_ = os.MkdirAll(libraryPath, 0755)
	createPayload := map[string]interface{}{
		"name":      "Library Batch",
		"mediaType": "book",
		"folders": []map[string]string{
			{"path": libraryPath},
		},
	}
	createBody, _ := json.Marshal(createPayload)
	req, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries", bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed to create library: %v", err)
	}
	var createdLib map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createdLib)
	resp.Body.Close()
	libID := createdLib["id"].(string)

	// Open DB to insert books, library items
	db, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	// Seed server settings to enable metadata.json creation
	_, err = db.Exec(`UPDATE settings SET value = '{"metadataMarkdownWithItem":true}' WHERE key = 'server-settings'`)
	if err != nil {
		t.Fatalf("Failed to enable metadataMarkdownWithItem settings: %v", err)
	}

	// Create physical directories for books to allow writing metadata.json
	book1Dir := filepath.Join(libraryPath, "BookOne")
	book2Dir := filepath.Join(libraryPath, "BookTwo")
	_ = os.MkdirAll(book1Dir, 0755)
	_ = os.MkdirAll(book2Dir, 0755)

	_, err = db.Exec(`INSERT INTO books (id, title, subtitle, publishedYear, publisher, explicit, abridged, narrators, tags, genres) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"book_1", "Original Book One", "", "", "Old Pub", 0, 0, "[]", "[]", "[]")
	if err != nil {
		t.Fatalf("Failed to insert book_1: %v", err)
	}
	_, err = db.Exec(`INSERT INTO libraryItems (id, libraryId, path, isFile, createdAt, updatedAt, mediaType, mediaId, title) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"item_1", libID, book1Dir, 0, "2026-06-08 12:00:00.000", "2026-06-08 12:00:00.000", "book", "book_1", "Original Book One")
	if err != nil {
		t.Fatalf("Failed to insert libraryItem1: %v", err)
	}

	_, err = db.Exec(`INSERT INTO books (id, title, subtitle, publishedYear, publisher, explicit, abridged, narrators, tags, genres) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"book_2", "Original Book Two", "", "", "Old Pub", 0, 0, "[]", "[]", "[]")
	if err != nil {
		t.Fatalf("Failed to insert book_2: %v", err)
	}
	_, err = db.Exec(`INSERT INTO libraryItems (id, libraryId, path, isFile, createdAt, updatedAt, mediaType, mediaId, title) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"item_2", libID, book2Dir, 0, "2026-06-08 12:00:00.000", "2026-06-08 12:00:00.000", "book", "book_2", "Original Book Two")
	if err != nil {
		t.Fatalf("Failed to insert libraryItem2: %v", err)
	}

	// 2. Call batch update endpoint
	newTags := []string{"Mystery", "Suspense"}
	newNarrators := []string{"Narrator X"}
	newPub := "Grand Publisher"
	newExplicit := true
	newAbridged := false

	batchPayload := []map[string]interface{}{
		{
			"id": "item_1",
			"mediaPayload": map[string]interface{}{
				"tags":      newTags,
				"narrators": newNarrators,
				"publisher": newPub,
				"explicit":  newExplicit,
				"abridged":  newAbridged,
			},
		},
		{
			"id": "item_2",
			"mediaPayload": map[string]interface{}{
				"tags":      newTags,
				"narrators": newNarrators,
				"publisher": newPub,
				"explicit":  newExplicit,
				"abridged":  newAbridged,
			},
		},
	}
	batchBody, _ := json.Marshal(batchPayload)
	req, _ = http.NewRequest("POST", h.BaseURL+"/api/items/batch/update", bytes.NewReader(batchBody))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Batch update request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 3. Verify DB changes
	var tagsRaw1, pub1 string
	var explicit1, abridged1 int
	err = db.QueryRow("SELECT tags, publisher, explicit, abridged FROM books WHERE id = 'book_1'").Scan(&tagsRaw1, &pub1, &explicit1, &abridged1)
	if err != nil {
		t.Fatalf("Failed to query book_1 from DB: %v", err)
	}

	if pub1 != "Grand Publisher" {
		t.Errorf("Expected publisher to be 'Grand Publisher', got %q", pub1)
	}
	if explicit1 != 1 {
		t.Errorf("Expected explicit to be 1, got %d", explicit1)
	}
	if abridged1 != 0 {
		t.Errorf("Expected abridged to be 0, got %d", abridged1)
	}

	var tags1 []string
	if err := json.Unmarshal([]byte(tagsRaw1), &tags1); err != nil {
		t.Fatalf("Failed to parse tags: %v", err)
	}
	if len(tags1) != 2 || tags1[0] != "Mystery" || tags1[1] != "Suspense" {
		t.Errorf("Expected tags to be ['Mystery', 'Suspense'], got %v", tags1)
	}

	// 4. Verify physical metadata.json is written
	metaPath1 := filepath.Join(book1Dir, "metadata.json")
	if _, err := os.Stat(metaPath1); os.IsNotExist(err) {
		t.Fatalf("metadata.json not written to path: %s", metaPath1)
	}
	metaContent, err := os.ReadFile(metaPath1)
	if err != nil {
		t.Fatalf("Failed to read metadata.json: %v", err)
	}
	var metaMap map[string]interface{}
	if err := json.Unmarshal(metaContent, &metaMap); err != nil {
		t.Fatalf("Failed to parse metadata.json: %v", err)
	}
	if metaMap["publisher"] != "Grand Publisher" {
		t.Errorf("Expected metadata.json publisher to be 'Grand Publisher', got %v", metaMap["publisher"])
	}
	if metaMap["explicit"] != true {
		t.Errorf("Expected metadata.json explicit to be true, got %v", metaMap["explicit"])
	}
}
