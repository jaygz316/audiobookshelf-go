package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

func BenchmarkOPDS_AuthorItems_Performance(b *testing.B) {
	db := setupTestDB(b)
	defer db.Close()

	// Drop and recreate tables to match OPDS structure
	queries := []string{
		"DROP TABLE IF EXISTS users",
		"CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT, email TEXT, pash TEXT, type TEXT, token TEXT, isActive INTEGER, isLocked INTEGER, lastSeen TEXT, permissions TEXT, bookmarks TEXT, extraData TEXT, createdAt TEXT, updatedAt TEXT)",
		"DROP TABLE IF EXISTS authors",
		"CREATE TABLE authors (id TEXT PRIMARY KEY, name TEXT, description TEXT, libraryId TEXT)",
		"DROP TABLE IF EXISTS bookAuthors",
		"CREATE TABLE bookAuthors (bookId TEXT, authorId TEXT)",
	}
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			b.Fatalf("Failed to execute setup query %q: %v", q, err)
		}
	}

	// Insert library
	_, err := db.Exec("INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, lastScan, lastScanVersion, settings, createdAt, updatedAt) VALUES ('lib-1', 'Audiobooks', 1, '', 'book', '', '2026-07-10T00:00:00Z', '', '{}', '2026-07-10T00:00:00Z', '2026-07-10T00:00:00Z')")
	if err != nil {
		b.Fatalf("Failed to insert library: %v", err)
	}

	// Insert author
	_, err = db.Exec("INSERT INTO authors (id, name, description, libraryId) VALUES ('author-1', 'Author One', 'Awesome Author', 'lib-1')")
	if err != nil {
		b.Fatalf("Failed to insert author: %v", err)
	}

	// Insert a regular user
	_, err = db.Exec(`
		INSERT INTO users (id, username, type, isActive, permissions) 
		VALUES ('user-regular', 'regular-user', 'user', 1, '{"accessAllLibraries": true, "accessAllTags": true, "accessExplicitContent": true}')
	`)
	if err != nil {
		b.Fatalf("Failed to insert regular user: %v", err)
	}

	// Insert N books to simulate a large author catalog
	numBooks := 100
	for i := 0; i < numBooks; i++ {
		itemID := fmt.Sprintf("item-%d", i)
		bookID := fmt.Sprintf("book-%d", i)
		title := fmt.Sprintf("Book %d", i)

		_, err = db.Exec(`
			INSERT INTO libraryItems (id, ino, libraryId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size, libraryFolderId, authorNamesFirstLast, authorNamesLastFirst, title, titleIgnorePrefix) 
			VALUES (?, ?, 'lib-1', ?, ?, 0, '123', '123', '123', '123', '1710000000000', 0, 0, 'book', ?, 500, '', 'Author One', 'Author One', ?, ?)
		`, itemID, itemID, "/books/"+itemID, itemID, bookID, title, title)
		if err != nil {
			b.Fatalf("Failed to insert library item: %v", err)
		}

		_, err = db.Exec(`
			INSERT INTO books (id, title, titleIgnorePrefix, subtitle, publishedYear, publishedDate, publisher, description, isbn, asin, language, explicit, abridged, coverPath, duration, narrators, audioFiles, ebookFile, chapters, tags, genres, lockedFields) 
			VALUES (?, ?, ?, '', '2025', '2025', 'Publisher X', 'A descriptive summary', '', '', 'en', 0, 0, '', 0.0, '[]', '[]', '{"ebookFormat": "epub"}', '[]', '[]', '[]', '[]')
		`, bookID, title, title)
		if err != nil {
			b.Fatalf("Failed to insert book: %v", err)
		}

		_, err = db.Exec("INSERT INTO bookAuthors (bookId, authorId) VALUES (?, 'author-1')", bookID)
		if err != nil {
			b.Fatalf("Failed to insert bookAuthor for %s: %v", bookID, err)
		}
	}

	handler := ServeOPDS(db)

	userSession, err := idb.GetUserByID(db, "user-regular")
	if err != nil {
		b.Fatalf("Failed to get user session: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1/authors/author-1", nil)
		ctx := context.WithValue(req.Context(), core.UserContextKey, userSession)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			b.Fatalf("Expected status 200, got %d", rr.Code)
		}
	}
}
