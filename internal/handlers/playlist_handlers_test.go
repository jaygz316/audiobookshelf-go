package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"audiobookshelf/internal/core"
	"audiobookshelf/internal/playlist"
)

func TestPlaylistHandlers(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Seed database with books/library data to prevent errors
	_, err := db.Exec(`INSERT INTO books (id, title) VALUES ('book-1', 'Book 1'), ('book-2', 'Book 2')`)
	if err != nil {
		t.Fatalf("failed to seed books: %v", err)
	}

	adminUser := &core.UserSession{
		ID:                 "user-admin",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}

	regularUser := &core.UserSession{
		ID:                 "user-regular",
		Username:           "regular",
		Type:               "user",
		IsActive:           true,
		AccessAllLibraries: true,
	}

	var createdPlaylistID string
	var createdCollectionID string

	// 1. Create Playlist (POST /api/playlists)
	t.Run("Create Playlist", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":  "My Test Playlist",
			"items": []string{"book-1"},
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/playlists", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, regularUser))
		rr := httptest.NewRecorder()

		handler := handleCreatePlaylist(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("Expected status 201 Created, got %d: %s", rr.Code, rr.Body.String())
		}

		var created playlist.Playlist
		if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if created.Name != "My Test Playlist" {
			t.Errorf("Expected playlist name 'My Test Playlist', got '%s'", created.Name)
		}
		if len(created.ItemIDs) != 1 || created.ItemIDs[0] != "book-1" {
			t.Errorf("Expected itemIds ['book-1'], got %v", created.ItemIDs)
		}
		createdPlaylistID = created.ID
	})

	// 2. Get Playlist by ID (GET /api/playlists/:id)
	t.Run("Get Playlist", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/playlists/"+createdPlaylistID, nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, regularUser))
		rr := httptest.NewRecorder()

		handler := handleGetPlaylist(db, createdPlaylistID)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected status 200 OK, got %d", rr.Code)
		}

		var retrieved playlist.Playlist
		if err := json.Unmarshal(rr.Body.Bytes(), &retrieved); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if retrieved.ID != createdPlaylistID {
			t.Errorf("Expected ID %s, got %s", createdPlaylistID, retrieved.ID)
		}
		if retrieved.Name != "My Test Playlist" {
			t.Errorf("Expected name 'My Test Playlist', got '%s'", retrieved.Name)
		}
	})

	// 3. Get Playlist (Forbidden check - regular user trying to access admin's private playlist if we change ownership)
	t.Run("Get Playlist - Forbidden check", func(t *testing.T) {
		// Update user ID of playlist to admin
		_, err := db.Exec("UPDATE playlists SET userId = 'user-admin' WHERE id = ?", createdPlaylistID)
		if err != nil {
			t.Fatalf("Failed to update playlist owner: %v", err)
		}

		req := httptest.NewRequest("GET", "/api/playlists/"+createdPlaylistID, nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, regularUser))
		rr := httptest.NewRecorder()

		handler := handleGetPlaylist(db, createdPlaylistID)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected status 403 Forbidden for non-owner, got %d", rr.Code)
		}

		// Revert owner
		_, err = db.Exec("UPDATE playlists SET userId = 'user-regular' WHERE id = ?", createdPlaylistID)
		if err != nil {
			t.Fatalf("Failed to revert playlist owner: %v", err)
		}
	})

	// 4. Update Playlist (PATCH /api/playlists/:id)
	t.Run("Update Playlist", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":  "My Updated Test Playlist",
			"items": []string{"book-1", "book-2"},
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("PATCH", "/api/playlists/"+createdPlaylistID, bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, regularUser))
		rr := httptest.NewRecorder()

		handler := handleUpdatePlaylist(db, createdPlaylistID)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected status 200 OK, got %d: %s", rr.Code, rr.Body.String())
		}

		var updated playlist.Playlist
		if err := json.Unmarshal(rr.Body.Bytes(), &updated); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if updated.Name != "My Updated Test Playlist" {
			t.Errorf("Expected name 'My Updated Test Playlist', got '%s'", updated.Name)
		}
		if len(updated.ItemIDs) != 2 || updated.ItemIDs[0] != "book-1" || updated.ItemIDs[1] != "book-2" {
			t.Errorf("Expected items ['book-1', 'book-2'], got %v", updated.ItemIDs)
		}
	})

	// 5. Get All Playlists (GET /api/playlists)
	t.Run("Get Playlists list", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/playlists", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, regularUser))
		rr := httptest.NewRecorder()

		handler := handleGetPlaylists(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected status 200 OK, got %d", rr.Code)
		}

		var resp map[string][]map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		playlists, ok := resp["playlists"]
		if !ok || len(playlists) != 1 {
			t.Fatalf("Expected exactly 1 playlist in response, got %d", len(playlists))
		}

		if playlists[0]["name"] != "My Updated Test Playlist" {
			t.Errorf("Expected name 'My Updated Test Playlist', got '%s'", playlists[0]["name"])
		}
	})

	// 6. Get Library Playlists (GET /api/libraries/:id/playlists)
	t.Run("Get Library Playlists", func(t *testing.T) {
		// Set libraryId on the playlist
		_, err := db.Exec("UPDATE playlists SET libraryId = 'lib-1' WHERE id = ?", createdPlaylistID)
		if err != nil {
			t.Fatalf("Failed to update libraryId: %v", err)
		}

		req := httptest.NewRequest("GET", "/api/libraries/lib-1/playlists", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, regularUser))
		rr := httptest.NewRecorder()

		handler := handleGetLibraryPlaylists(db, "lib-1")
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected status 200 OK, got %d", rr.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		results := resp["results"].([]interface{})
		if len(results) != 1 {
			t.Fatalf("Expected exactly 1 library playlist in response, got %d", len(results))
		}
	})

	// 7. Create Collection (POST /api/collections)
	t.Run("Create Collection - Regular user blocked", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":        "Fantasy Collection",
			"description": "Awesome fantasy series",
			"libraryId":   "lib-1",
			"books":       []string{"book-1"},
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/collections", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, regularUser))
		rr := httptest.NewRecorder()

		handler := handleCreateCollection(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected status 403 Forbidden, got %d", rr.Code)
		}
	})

	t.Run("Create Collection - Admin user success", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":        "Fantasy Collection",
			"description": "Awesome fantasy series",
			"libraryId":   "lib-1",
			"books":       []string{"book-1"},
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/collections", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminUser))
		rr := httptest.NewRecorder()

		handler := handleCreateCollection(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("Expected status 201 Created, got %d: %s", rr.Code, rr.Body.String())
		}

		var created playlist.Collection
		if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if created.Name != "Fantasy Collection" {
			t.Errorf("Expected collection name 'Fantasy Collection', got '%s'", created.Name)
		}
		if created.Description != "Awesome fantasy series" {
			t.Errorf("Expected description 'Awesome fantasy series', got '%s'", created.Description)
		}
		createdCollectionID = created.ID
	})

	// 8. Get Collection (GET /api/collections/:id)
	t.Run("Get Collection", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/collections/"+createdCollectionID, nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, regularUser))
		rr := httptest.NewRecorder()

		handler := handleGetCollection(db, createdCollectionID)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected status 200 OK, got %d", rr.Code)
		}

		var retrieved playlist.Collection
		if err := json.Unmarshal(rr.Body.Bytes(), &retrieved); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if retrieved.ID != createdCollectionID {
			t.Errorf("Expected ID %s, got %s", createdCollectionID, retrieved.ID)
		}
		if retrieved.Name != "Fantasy Collection" {
			t.Errorf("Expected name 'Fantasy Collection', got '%s'", retrieved.Name)
		}
	})

	// 9. Update Collection (PATCH /api/collections/:id)
	t.Run("Update Collection", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":        "Epic Fantasy Collection",
			"description": "Better fantasy books",
			"books":       []string{"book-1", "book-2"},
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("PATCH", "/api/collections/"+createdCollectionID, bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminUser))
		rr := httptest.NewRecorder()

		handler := handleUpdateCollection(db, createdCollectionID)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected status 200 OK, got %d: %s", rr.Code, rr.Body.String())
		}

		var updated playlist.Collection
		if err := json.Unmarshal(rr.Body.Bytes(), &updated); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if updated.Name != "Epic Fantasy Collection" {
			t.Errorf("Expected name 'Epic Fantasy Collection', got '%s'", updated.Name)
		}
		if updated.Description != "Better fantasy books" {
			t.Errorf("Expected description 'Better fantasy books', got '%s'", updated.Description)
		}
		if len(updated.ItemIDs) != 2 || updated.ItemIDs[0] != "book-1" || updated.ItemIDs[1] != "book-2" {
			t.Errorf("Expected book IDs ['book-1', 'book-2'], got %v", updated.ItemIDs)
		}
	})

	// 10. Get All Collections (GET /api/collections)
	t.Run("Get All Collections", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/collections", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, regularUser))
		rr := httptest.NewRecorder()

		handler := handleGetCollections(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected status 200 OK, got %d", rr.Code)
		}

		var resp map[string][]map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		collections, ok := resp["collections"]
		if !ok || len(collections) != 1 {
			t.Fatalf("Expected exactly 1 collection in response, got %d", len(collections))
		}

		if collections[0]["name"] != "Epic Fantasy Collection" {
			t.Errorf("Expected name 'Epic Fantasy Collection', got '%s'", collections[0]["name"])
		}
	})

	// 11. Get Library Collections (GET /api/libraries/:id/collections)
	t.Run("Get Library Collections", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/libraries/lib-1/collections", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, regularUser))
		rr := httptest.NewRecorder()

		handler := handleGetLibraryCollections(db, "lib-1")
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected status 200 OK, got %d", rr.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		results := resp["results"].([]interface{})
		if len(results) != 1 {
			t.Fatalf("Expected exactly 1 library collection in response, got %d", len(results))
		}
	})

	// 12. Delete Playlist (DELETE /api/playlists/:id)
	t.Run("Delete Playlist", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/playlists/"+createdPlaylistID, nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, regularUser))
		rr := httptest.NewRecorder()

		handler := handleDeletePlaylist(db, createdPlaylistID)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected status 200 OK, got %d", rr.Code)
		}

		// Verify 404 on GET now
		reqGet := httptest.NewRequest("GET", "/api/playlists/"+createdPlaylistID, nil)
		reqGet = reqGet.WithContext(context.WithValue(reqGet.Context(), core.UserContextKey, regularUser))
		rrGet := httptest.NewRecorder()

		handlerGet := handleGetPlaylist(db, createdPlaylistID)
		handlerGet.ServeHTTP(rrGet, reqGet)

		if rrGet.Code != http.StatusNotFound {
			t.Errorf("Expected status 404 Not Found after deletion, got %d", rrGet.Code)
		}
	})

	// 13. Delete Collection (DELETE /api/collections/:id)
	t.Run("Delete Collection", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/collections/"+createdCollectionID, nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminUser))
		rr := httptest.NewRecorder()

		handler := handleDeleteCollection(db, createdCollectionID)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected status 200 OK, got %d", rr.Code)
		}

		// Verify 404 on GET now
		reqGet := httptest.NewRequest("GET", "/api/collections/"+createdCollectionID, nil)
		reqGet = reqGet.WithContext(context.WithValue(reqGet.Context(), core.UserContextKey, regularUser))
		rrGet := httptest.NewRecorder()

		handlerGet := handleGetCollection(db, createdCollectionID)
		handlerGet.ServeHTTP(rrGet, reqGet)

		if rrGet.Code != http.StatusNotFound {
			t.Errorf("Expected status 404 Not Found after deletion, got %d", rrGet.Code)
		}
	})
}
