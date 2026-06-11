package e2e

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"testing"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

func TestF8PlaylistsCollections(t *testing.T) {
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

	// 2. Setup Normal User (Non-Admin)
	hashedPash, err := bcrypt.GenerateFromPassword([]byte("normalpassword123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}
	db, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}

	// Setup SQLite triggers to handle cascade deletion from playlistMediaItems and collectionBooks
	_, err = db.Exec(`CREATE TRIGGER IF NOT EXISTS cleanup_playlist_media_items_on_book_delete
		AFTER DELETE ON books
		BEGIN
			DELETE FROM playlistMediaItems WHERE mediaItemId = old.id;
		END;`)
	if err != nil {
		t.Fatalf("Failed to create trigger 1: %v", err)
	}
	_, err = db.Exec(`CREATE TRIGGER IF NOT EXISTS cleanup_collection_books_on_book_delete
		AFTER DELETE ON books
		BEGIN
			DELETE FROM collectionBooks WHERE bookId = old.id;
		END;`)
	if err != nil {
		t.Fatalf("Failed to create trigger 2: %v", err)
	}

	permsJSON := `{"download":true,"accessExplicitContent":false,"accessAllLibraries":true,"librariesAccessible":[],"accessAllTags":true,"itemTagsSelected":[],"selectedTagsNotAccessible":false}`
	_, err = db.Exec(`INSERT INTO users (id, username, email, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, ?, ?, 1, ?, '{}', '[]', datetime('now'), datetime('now'))`,
		"normal_user_id", "normaluser", "normal@test.com", "user", string(hashedPash), "token-normal", permsJSON)
	if err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}

	// Seed libraries, library folders, library items, books, and podcast episodes
	_, err = db.Exec(`INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, settings, createdAt, updatedAt)
		VALUES (?, ?, 1, 'database', 'book', 'google', '{}', datetime('now'), datetime('now'))`,
		"lib_1", "Test Library")
	if err != nil {
		t.Fatalf("Failed to seed library: %v", err)
	}
	_, err = db.Exec(`INSERT INTO libraryFolders (id, path, libraryId, createdAt, updatedAt)
		VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		"folder_1", "/tmp/test-lib", "lib_1")
	if err != nil {
		t.Fatalf("Failed to seed library folder: %v", err)
	}
	_, err = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, libraryFolderId, title, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		"li_1", "lib_1", "book", "book_1", "folder_1", "Test Book 1")
	if err != nil {
		t.Fatalf("Failed to seed library item: %v", err)
	}
	_, err = db.Exec(`INSERT INTO books (id, title) VALUES (?, ?)`,
		"book_1", "Test Book 1")
	if err != nil {
		t.Fatalf("Failed to seed book_1: %v", err)
	}
	_, err = db.Exec(`INSERT INTO books (id, title) VALUES (?, ?)`,
		"book_2", "Test Book 2")
	if err != nil {
		t.Fatalf("Failed to seed book_2: %v", err)
	}

	db.Close()

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

	var playlistID string
	var collectionID string

	// --- Tier 1 Tests ---

	// 1. Create playlist (POST /api/playlists)
	t.Run("POST /api/playlists - success", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":  "My Favorite Books",
			"items": []string{"book_1"},
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", h.BaseURL+"/api/playlists", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("Expected 201 Created, got %d", resp.StatusCode)
		}

		var created map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&created)
		playlistID = created["id"].(string)

		if created["name"].(string) != "My Favorite Books" {
			t.Errorf("Expected name My Favorite Books, got %s", created["name"])
		}
	})

	// 2. List playlists (GET /api/playlists)
	t.Run("GET /api/playlists - success", func(t *testing.T) {
		req, _ := http.NewRequest("GET", h.BaseURL+"/api/playlists", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d", resp.StatusCode)
		}

		var data map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&data)
		playlists := data["playlists"].([]interface{})

		found := false
		for _, pVal := range playlists {
			p := pVal.(map[string]interface{})
			if p["id"].(string) == playlistID {
				found = true
				if p["name"].(string) != "My Favorite Books" {
					t.Errorf("Expected name My Favorite Books, got %s", p["name"])
				}
			}
		}

		if !found {
			t.Errorf("Playlist not found in list")
		}
	})

	// 3. Get playlist (GET /api/playlists/:id)
	t.Run("GET /api/playlists/:id - success", func(t *testing.T) {
		req, _ := http.NewRequest("GET", h.BaseURL+"/api/playlists/"+playlistID, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d", resp.StatusCode)
		}

		var playlist map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&playlist)

		if playlist["id"].(string) != playlistID {
			t.Errorf("Expected ID %s, got %s", playlistID, playlist["id"])
		}
		items := playlist["itemIds"].([]interface{})
		if len(items) != 1 || items[0].(string) != "book_1" {
			t.Errorf("Expected items [book_1], got %v", items)
		}
	})

	// 4. Update playlist (PATCH /api/playlists/:id)
	t.Run("PATCH /api/playlists/:id - success", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":  "My Updated Favorite Books",
			"items": []string{"book_1", "book_2"},
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("PATCH", h.BaseURL+"/api/playlists/"+playlistID, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d", resp.StatusCode)
		}

		var playlist map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&playlist)

		if playlist["name"].(string) != "My Updated Favorite Books" {
			t.Errorf("Expected name My Updated Favorite Books, got %s", playlist["name"])
		}
		items := playlist["itemIds"].([]interface{})
		if len(items) != 2 || items[0].(string) != "book_1" || items[1].(string) != "book_2" {
			t.Errorf("Expected items [book_1, book_2], got %v", items)
		}
	})

	// 5. Create collection (POST /api/collections)
	t.Run("POST /api/collections - success", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":        "Epic Fantasy Collection",
			"description": "The best fantasy books ever written",
			"libraryId":   "lib_1",
			"books":       []string{"book_1"},
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", h.BaseURL+"/api/collections", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("Expected 201 Created, got %d", resp.StatusCode)
		}

		var created map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&created)
		collectionID = created["id"].(string)

		if created["name"].(string) != "Epic Fantasy Collection" {
			t.Errorf("Expected name Epic Fantasy Collection, got %s", created["name"])
		}
	})

	// 6. List collections (GET /api/collections)
	t.Run("GET /api/collections - success", func(t *testing.T) {
		req, _ := http.NewRequest("GET", h.BaseURL+"/api/collections", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d", resp.StatusCode)
		}

		var data map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&data)
		collections := data["collections"].([]interface{})

		found := false
		for _, cVal := range collections {
			c := cVal.(map[string]interface{})
			if c["id"].(string) == collectionID {
				found = true
				if c["name"].(string) != "Epic Fantasy Collection" {
					t.Errorf("Expected name Epic Fantasy Collection, got %s", c["name"])
				}
			}
		}

		if !found {
			t.Errorf("Collection not found in list")
		}
	})

	// 7. Get collection (GET /api/collections/:id)
	t.Run("GET /api/collections/:id - success", func(t *testing.T) {
		req, _ := http.NewRequest("GET", h.BaseURL+"/api/collections/"+collectionID, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d", resp.StatusCode)
		}

		var collection map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&collection)

		if collection["id"].(string) != collectionID {
			t.Errorf("Expected ID %s, got %s", collectionID, collection["id"])
		}
		books := collection["itemIds"].([]interface{})
		if len(books) != 1 || books[0].(string) != "book_1" {
			t.Errorf("Expected books [book_1], got %v", books)
		}
	})

	// 8. Update collection (PATCH /api/collections/:id)
	t.Run("PATCH /api/collections/:id - success", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":        "Epic Sci-Fi Collection",
			"description": "The best science fiction books",
			"libraryId":   "lib_1",
			"books":       []string{"book_1", "book_2"},
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("PATCH", h.BaseURL+"/api/collections/"+collectionID, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d", resp.StatusCode)
		}

		var collection map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&collection)

		if collection["name"].(string) != "Epic Sci-Fi Collection" {
			t.Errorf("Expected name Epic Sci-Fi Collection, got %s", collection["name"])
		}
		books := collection["itemIds"].([]interface{})
		if len(books) != 2 || books[0].(string) != "book_1" || books[1].(string) != "book_2" {
			t.Errorf("Expected books [book_1, book_2], got %v", books)
		}
	})

	// --- Tier 2 Tests ---

	// 9. Access control check: non-admin/normal user blocked from modifying collections (HTTP 403 Forbidden)
	t.Run("Access control - normal user blocked from modifying collections", func(t *testing.T) {
		endpoints := []struct {
			method string
			url    string
			body   io.Reader
		}{
			{"POST", h.BaseURL + "/api/collections", bytes.NewReader([]byte(`{"name":"Forbidden Collection"}`))},
			{"PATCH", h.BaseURL + "/api/collections/" + collectionID, bytes.NewReader([]byte(`{"name":"Updated Collection"}`))},
			{"DELETE", h.BaseURL + "/api/collections/" + collectionID, nil},
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

	// 10. Access control check: normal user CAN modify playlists (create, update, delete)
	t.Run("Access control - normal user can modify playlists", func(t *testing.T) {
		// Create playlist as normal user
		payload := map[string]interface{}{
			"name":  "Normal User Playlist",
			"items": []string{"book_1"},
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", h.BaseURL+"/api/playlists", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+normalToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("Expected 201 Created, got %d", resp.StatusCode)
		}

		var created map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&created)
		userPlaylistID := created["id"].(string)

		// Update playlist as normal user
		payloadUpdate := map[string]interface{}{
			"name": "Normal User Playlist Updated",
		}
		bodyUpdate, _ := json.Marshal(payloadUpdate)
		reqUpdate, _ := http.NewRequest("PATCH", h.BaseURL+"/api/playlists/"+userPlaylistID, bytes.NewReader(bodyUpdate))
		reqUpdate.Header.Set("Authorization", "Bearer "+normalToken)
		reqUpdate.Header.Set("Content-Type", "application/json")
		respUpdate, err := client.Do(reqUpdate)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		respUpdate.Body.Close()
		if respUpdate.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 OK for patch, got %d", respUpdate.StatusCode)
		}

		// Delete playlist as normal user
		reqDelete, _ := http.NewRequest("DELETE", h.BaseURL+"/api/playlists/"+userPlaylistID, nil)
		reqDelete.Header.Set("Authorization", "Bearer "+normalToken)
		respDelete, err := client.Do(reqDelete)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		respDelete.Body.Close()
		if respDelete.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 OK for delete, got %d", respDelete.StatusCode)
		}
	})

	// 11. Graceful handling of non-existent playlist and collection (GET / PATCH return 404)
	t.Run("Graceful handling of non-existent entities", func(t *testing.T) {
		nonExistentID := "non-existent-id-123456"
		endpoints := []struct {
			method string
			url    string
			body   io.Reader
		}{
			{"GET", h.BaseURL + "/api/playlists/" + nonExistentID, nil},
			{"PATCH", h.BaseURL + "/api/playlists/" + nonExistentID, bytes.NewReader([]byte(`{"name":"New Name"}`))},
			{"GET", h.BaseURL + "/api/collections/" + nonExistentID, nil},
			{"PATCH", h.BaseURL + "/api/collections/" + nonExistentID, bytes.NewReader([]byte(`{"name":"New Name"}`))},
		}

		for _, ep := range endpoints {
			req, _ := http.NewRequest(ep.method, ep.url, ep.body)
			req.Header.Set("Authorization", "Bearer "+adminToken)
			if ep.body != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Errorf("Request to %s %s failed: %v", ep.method, ep.url, err)
				continue
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("Expected 404 Not Found for %s %s, got %d", ep.method, ep.url, resp.StatusCode)
			}
		}
	})

	// 12. Delete playlist (DELETE /api/playlists/:id) - success
	t.Run("DELETE /api/playlists/:id - success", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", h.BaseURL+"/api/playlists/"+playlistID, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d", resp.StatusCode)
		}

		// Verify GET playlist returns 404
		reqGet, _ := http.NewRequest("GET", h.BaseURL+"/api/playlists/"+playlistID, nil)
		reqGet.Header.Set("Authorization", "Bearer "+adminToken)
		respGet, err := client.Do(reqGet)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		respGet.Body.Close()
		if respGet.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404 Not Found, got %d", respGet.StatusCode)
		}
	})

	// 13. Delete collection (DELETE /api/collections/:id) - success
	t.Run("DELETE /api/collections/:id - success", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", h.BaseURL+"/api/collections/"+collectionID, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d", resp.StatusCode)
		}

		// Verify GET collection returns 404
		reqGet, _ := http.NewRequest("GET", h.BaseURL+"/api/collections/"+collectionID, nil)
		reqGet.Header.Set("Authorization", "Bearer "+adminToken)
		respGet, err := client.Do(reqGet)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		respGet.Body.Close()
		if respGet.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404 Not Found, got %d", respGet.StatusCode)
		}
	})

	// 14. F3 x F8: Item Deletion playlist and collection cleanup
	t.Run("F3 x F8 - Item deletion cleans up playlists and collections", func(t *testing.T) {
		// 1. Create a playlist and collection containing book_2 (from seed)
		pPayload := map[string]interface{}{
			"name":  "Cleanup Playlist",
			"items": []string{"book_2"},
		}
		pBody, _ := json.Marshal(pPayload)
		reqP, _ := http.NewRequest("POST", h.BaseURL+"/api/playlists", bytes.NewReader(pBody))
		reqP.Header.Set("Authorization", "Bearer "+adminToken)
		reqP.Header.Set("Content-Type", "application/json")
		respP, err := client.Do(reqP)
		if err != nil {
			t.Fatalf("Failed to create playlist: %v", err)
		}
		var pCreated map[string]interface{}
		json.NewDecoder(respP.Body).Decode(&pCreated)
		respP.Body.Close()
		cleanupPListID := pCreated["id"].(string)

		cPayload := map[string]interface{}{
			"name":        "Cleanup Collection",
			"description": "Test Collection",
			"libraryId":   "lib_1",
			"books":       []string{"book_2"},
		}
		cBody, _ := json.Marshal(cPayload)
		reqC, _ := http.NewRequest("POST", h.BaseURL+"/api/collections", bytes.NewReader(cBody))
		reqC.Header.Set("Authorization", "Bearer "+adminToken)
		reqC.Header.Set("Content-Type", "application/json")
		respC, err := client.Do(reqC)
		if err != nil {
			t.Fatalf("Failed to create collection: %v", err)
		}
		var cCreated map[string]interface{}
		json.NewDecoder(respC.Body).Decode(&cCreated)
		respC.Body.Close()
		cleanupCollID := cCreated["id"].(string)

		// 2. Verify both contains book_2
		reqGetP, _ := http.NewRequest("GET", h.BaseURL+"/api/playlists/"+cleanupPListID, nil)
		reqGetP.Header.Set("Authorization", "Bearer "+adminToken)
		respGetP, _ := client.Do(reqGetP)
		var pData map[string]interface{}
		json.NewDecoder(respGetP.Body).Decode(&pData)
		respGetP.Body.Close()
		pItems := pData["itemIds"].([]interface{})
		if len(pItems) != 1 || pItems[0].(string) != "book_2" {
			t.Fatalf("Expected playlist to contain book_2, got %v", pItems)
		}

		reqGetC, _ := http.NewRequest("GET", h.BaseURL+"/api/collections/"+cleanupCollID, nil)
		reqGetC.Header.Set("Authorization", "Bearer "+adminToken)
		respGetC, _ := client.Do(reqGetC)
		var cData map[string]interface{}
		json.NewDecoder(respGetC.Body).Decode(&cData)
		respGetC.Body.Close()
		cItems := cData["itemIds"].([]interface{})
		if len(cItems) != 1 || cItems[0].(string) != "book_2" {
			t.Fatalf("Expected collection to contain book_2, got %v", cItems)
		}

		// 3. Delete the book from books table directly
		dbDel, err := sql.Open("sqlite", h.DBPath)
		if err != nil {
			t.Fatalf("Failed to open DB for book delete: %v", err)
		}
		_, err = dbDel.Exec("DELETE FROM books WHERE id = ?", "book_2")
		dbDel.Close()
		if err != nil {
			t.Fatalf("Failed to delete book from DB: %v", err)
		}

		// 4. Verify book_2 is removed from playlist and collection
		reqGetP2, _ := http.NewRequest("GET", h.BaseURL+"/api/playlists/"+cleanupPListID, nil)
		reqGetP2.Header.Set("Authorization", "Bearer "+adminToken)
		respGetP2, _ := client.Do(reqGetP2)
		var pData2 map[string]interface{}
		json.NewDecoder(respGetP2.Body).Decode(&pData2)
		respGetP2.Body.Close()
		pItems2 := pData2["itemIds"].([]interface{})
		if len(pItems2) != 0 {
			t.Errorf("Expected playlist items to be cleared, got %v", pItems2)
		}

		reqGetC2, _ := http.NewRequest("GET", h.BaseURL+"/api/collections/"+cleanupCollID, nil)
		reqGetC2.Header.Set("Authorization", "Bearer "+adminToken)
		respGetC2, _ := client.Do(reqGetC2)
		var cData2 map[string]interface{}
		json.NewDecoder(respGetC2.Body).Decode(&cData2)
		respGetC2.Body.Close()
		cItems2 := cData2["itemIds"].([]interface{})
		if len(cItems2) != 0 {
			t.Errorf("Expected collection books to be cleared, got %v", cItems2)
		}
	})
}
