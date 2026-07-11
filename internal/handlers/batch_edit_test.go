package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"audiobookshelf/internal/core"
	"audiobookshelf/internal/utils"
)

func TestHandleBatchUpdateLibraryItems(t *testing.T) {
	oldMetaPath := MetadataPath
	MetadataPath = t.TempDir()
	defer func() { MetadataPath = oldMetaPath }()

	db := setupTestDBShared(t)
	defer db.Close()

	// Insert mock library
	_, err := db.Exec(`INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, settings, createdAt, updatedAt) VALUES ('lib1', 'Audiobooks', 1, 'book', 'book', 'local', '{}', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}

	// Insert book 1
	_, err = db.Exec(`INSERT INTO books (id, title, subtitle, publishedYear, publisher, explicit, abridged, narrators, tags, genres) VALUES ('book1', 'Book One', 'Sub One', '2020', 'Pub A', 0, 0, '[]', '[]', '[]')`)
	if err != nil {
		t.Fatalf("Failed to insert book1: %v", err)
	}
	_, err = db.Exec(`INSERT INTO libraryItems (id, libraryId, createdAt, updatedAt, mediaType, mediaId, title) VALUES ('item1', 'lib1', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', 'book', 'book1', 'Book One')`)
	if err != nil {
		t.Fatalf("Failed to insert libraryItem1: %v", err)
	}

	// Insert book 2
	_, err = db.Exec(`INSERT INTO books (id, title, subtitle, publishedYear, publisher, explicit, abridged, narrators, tags, genres) VALUES ('book2', 'Book Two', 'Sub Two', '2021', 'Pub B', 0, 0, '[]', '[]', '[]')`)
	if err != nil {
		t.Fatalf("Failed to insert book2: %v", err)
	}
	_, err = db.Exec(`INSERT INTO libraryItems (id, libraryId, createdAt, updatedAt, mediaType, mediaId, title) VALUES ('item2', 'lib1', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', 'book', 'book2', 'Book Two')`)
	if err != nil {
		t.Fatalf("Failed to insert libraryItem2: %v", err)
	}

	cfg := &core.Config{
		RouterBasePath: "/",
	}

	handler := handleBatchUpdateLibraryItems(db, cfg)

	// Case 1: Unauthorized user (not admin/root)
	{
		payload := []BatchUpdateItem{
			{
				ID: "item1",
				MediaPayload: BatchUpdateMediaPayload{
					Publisher: utils.NullIfEmpty("New Pub"),
				},
			},
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/items/batch/update", bytes.NewReader(body))
		user := &core.UserSession{
			ID:       "user1",
			Username: "user",
			Type:     "user",
		}
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden for non-admin user, got %d", rr.Code)
		}
	}

	// Case 2: Batch update tags, publisher, and authors for book1 and book2
	{
		newTags := []string{"Sci-Fi", "Classic"}
		newAuthors := []string{"Author X"}
		payload := []BatchUpdateItem{
			{
				ID: "item1",
				MediaPayload: BatchUpdateMediaPayload{
					Tags:      &newTags,
					Publisher: utils.NullIfEmpty("Super Publisher"),
					Authors:   &newAuthors,
				},
			},
			{
				ID: "item2",
				MediaPayload: BatchUpdateMediaPayload{
					Tags:      &newTags,
					Publisher: utils.NullIfEmpty("Super Publisher"),
					Authors:   &newAuthors,
				},
			},
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/items/batch/update", bytes.NewReader(body))
		user := &core.UserSession{
			ID:       "user1",
			Username: "admin",
			Type:     "admin",
		}
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		// Verify updates in DB
		var pub1, tagsRaw1 string
		err = db.QueryRow("SELECT publisher, tags FROM books WHERE id = 'book1'").Scan(&pub1, &tagsRaw1)
		if err != nil {
			t.Fatalf("Query book1 failed: %v", err)
		}
		if pub1 != "Super Publisher" {
			t.Errorf("Expected book1 publisher to be 'Super Publisher', got %q", pub1)
		}
		var tags1 []string
		_ = json.Unmarshal([]byte(tagsRaw1), &tags1)
		if len(tags1) != 2 || tags1[0] != "Sci-Fi" || tags1[1] != "Classic" {
			t.Errorf("Expected book1 tags to be ['Sci-Fi', 'Classic'], got %v", tags1)
		}

		// Verify author names set on library items
		var authorNames1 string
		err = db.QueryRow("SELECT authorNamesFirstLast FROM libraryItems WHERE id = 'item1'").Scan(&authorNames1)
		if err != nil {
			t.Fatalf("Query libraryItem1 failed: %v", err)
		}
		if authorNames1 != "Author X" {
			t.Errorf("Expected libraryItem1 authorNamesFirstLast to be 'Author X', got %q", authorNames1)
		}
	}
}
