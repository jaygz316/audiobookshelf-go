package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestOPDS_Adversarial_Permissions_Extended(t *testing.T) {
	db := prepareOPDSTestDB(t)
	defer db.Close()

	hashed, err := bcrypt.GenerateFromPassword([]byte("mypassword"), 8)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	// ----------------------------------------------------
	// 1. Setup Data for Explicit Content Restrictions test
	// ----------------------------------------------------
	// Explicit Book
	_, err = db.Exec(`
		INSERT INTO libraryItems (id, ino, libraryId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size, libraryFolderId, authorNamesFirstLast, authorNamesLastFirst, title, titleIgnorePrefix) 
		VALUES ('item-explicit', '124', 'lib-1', '/books/book-explicit', 'book-explicit', 0, '123', '123', '123', '123', '1710000000000', 0, 0, 'book', 'book-explicit', 500, '', 'Author Explicit', 'Author Explicit', 'Explicit Title Book', 'Explicit Title Book')
	`)
	if err != nil {
		t.Fatalf("Failed to insert explicit library item: %v", err)
	}
	_, err = db.Exec(`
		INSERT INTO books (id, title, titleIgnorePrefix, subtitle, publishedYear, publishedDate, publisher, description, isbn, asin, language, explicit, abridged, coverPath, duration, narrators, audioFiles, ebookFile, chapters, tags, genres, lockedFields) 
		VALUES ('book-explicit', 'Explicit Title Book', 'Explicit Title Book', '', '2025', '2025', 'Publisher X', 'Explicit Book Description', '', '', 'en', 1, 0, '', 0.0, '[]', '[]', '{"ebookFormat": "epub"}', '[]', '[]', '[]', '[]')
	`)
	if err != nil {
		t.Fatalf("Failed to insert explicit book: %v", err)
	}
	// Explicit Author
	_, err = db.Exec(`INSERT INTO authors (id, name, description, libraryId) VALUES ('author-explicit', 'Author Explicit', 'Explicit Author', 'lib-1')`)
	if err != nil {
		t.Fatalf("Failed to insert explicit author: %v", err)
	}
	_, err = db.Exec(`INSERT INTO bookAuthors (bookId, authorId) VALUES ('book-explicit', 'author-explicit')`)
	if err != nil {
		t.Fatalf("Failed to link explicit book to author: %v", err)
	}
	// Explicit Series
	_, err = db.Exec(`INSERT INTO series (id, name, description, libraryId) VALUES ('series-explicit', 'Series Explicit', 'Explicit Series', 'lib-1')`)
	if err != nil {
		t.Fatalf("Failed to insert explicit series: %v", err)
	}
	_, err = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('book-explicit', 'series-explicit', '1.0')`)
	if err != nil {
		t.Fatalf("Failed to link explicit book to series: %v", err)
	}
	// Explicit Collection
	_, err = db.Exec(`INSERT INTO collections (id, name, description, libraryId) VALUES ('coll-explicit', 'Collection Explicit', 'Explicit Collection', 'lib-1')`)
	if err != nil {
		t.Fatalf("Failed to insert explicit collection: %v", err)
	}
	_, err = db.Exec(`INSERT INTO collectionBooks (id, "order", bookId, collectionId) VALUES ('cb-explicit', 1, 'book-explicit', 'coll-explicit')`)
	if err != nil {
		t.Fatalf("Failed to link explicit book to collection: %v", err)
	}

	// ----------------------------------------------------
	// 2. Setup Data for Selected Tags Not Accessible test
	// ----------------------------------------------------
	// Banned Book (with tag "banned")
	bannedTags, _ := json.Marshal([]string{"banned"})
	_, err = db.Exec(`
		INSERT INTO libraryItems (id, ino, libraryId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size, libraryFolderId, authorNamesFirstLast, authorNamesLastFirst, title, titleIgnorePrefix) 
		VALUES ('item-banned', '125', 'lib-1', '/books/book-banned', 'book-banned', 0, '123', '123', '123', '123', '1710000000000', 0, 0, 'book', 'book-banned', 500, '', 'Author Banned', 'Author Banned', 'Banned Title Book', 'Banned Title Book')
	`)
	if err != nil {
		t.Fatalf("Failed to insert banned library item: %v", err)
	}
	_, err = db.Exec(`
		INSERT INTO books (id, title, titleIgnorePrefix, subtitle, publishedYear, publishedDate, publisher, description, isbn, asin, language, explicit, abridged, coverPath, duration, narrators, audioFiles, ebookFile, chapters, tags, genres, lockedFields) 
		VALUES ('book-banned', 'Banned Title Book', 'Banned Title Book', '', '2025', '2025', 'Publisher X', 'Banned Book Description', '', '', 'en', 0, 0, '', 0.0, '[]', '[]', '{"ebookFormat": "epub"}', '[]', ?, '[]', '[]')
	`, bannedTags)
	if err != nil {
		t.Fatalf("Failed to insert banned book: %v", err)
	}
	// Banned Author
	_, err = db.Exec(`INSERT INTO authors (id, name, description, libraryId) VALUES ('author-banned', 'Author Banned', 'Banned Author', 'lib-1')`)
	if err != nil {
		t.Fatalf("Failed to insert banned author: %v", err)
	}
	_, err = db.Exec(`INSERT INTO bookAuthors (bookId, authorId) VALUES ('book-banned', 'author-banned')`)
	if err != nil {
		t.Fatalf("Failed to link banned book to author: %v", err)
	}
	// Banned Series
	_, err = db.Exec(`INSERT INTO series (id, name, description, libraryId) VALUES ('series-banned', 'Series Banned', 'Banned Series', 'lib-1')`)
	if err != nil {
		t.Fatalf("Failed to insert banned series: %v", err)
	}
	_, err = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('book-banned', 'series-banned', '1.0')`)
	if err != nil {
		t.Fatalf("Failed to link banned book to series: %v", err)
	}
	// Banned Collection
	_, err = db.Exec(`INSERT INTO collections (id, name, description, libraryId) VALUES ('coll-banned', 'Collection Banned', 'Banned Collection', 'lib-1')`)
	if err != nil {
		t.Fatalf("Failed to insert banned collection: %v", err)
	}
	_, err = db.Exec(`INSERT INTO collectionBooks (id, "order", bookId, collectionId) VALUES ('cb-banned', 1, 'book-banned', 'coll-banned')`)
	if err != nil {
		t.Fatalf("Failed to link banned book to collection: %v", err)
	}

	// ----------------------------------------------------
	// 3. Create Restricted Users
	// ----------------------------------------------------
	// User 1: No Explicit Content
	_, err = db.Exec(`
		INSERT INTO users (id, username, type, pash, isActive, permissions) 
		VALUES ('user-no-explicit', 'no-explicit-user', 'user', ?, 1, ?)
	`, string(hashed), `{"accessAllLibraries": true, "accessAllTags": true, "accessExplicitContent": false}`)
	if err != nil {
		t.Fatalf("Failed to insert user-no-explicit: %v", err)
	}
	// Playlists for explicit user
	_, err = db.Exec(`INSERT INTO playlists (id, name, description, libraryId, userId) VALUES ('play-explicit', 'Explicit Playlist', '', 'lib-1', 'user-no-explicit')`)
	if err != nil {
		t.Fatalf("Failed to insert explicit playlist: %v", err)
	}
	_, err = db.Exec(`INSERT INTO playlistMediaItems (id, mediaItemId, mediaItemType, "order", playlistId) VALUES ('pmi-explicit', 'item-explicit', 'book', 1, 'play-explicit')`)
	if err != nil {
		t.Fatalf("Failed to insert explicit playlist media item: %v", err)
	}

	// User 2: Banned Tag Excluded
	_, err = db.Exec(`
		INSERT INTO users (id, username, type, pash, isActive, permissions) 
		VALUES ('user-exclude-banned', 'exclude-banned-user', 'user', ?, 1, ?)
	`, string(hashed), `{"accessAllLibraries": true, "accessAllTags": false, "itemTagsSelected": ["banned"], "selectedTagsNotAccessible": true, "accessExplicitContent": true}`)
	if err != nil {
		t.Fatalf("Failed to insert user-exclude-banned: %v", err)
	}
	// Playlists for banned user
	_, err = db.Exec(`INSERT INTO playlists (id, name, description, libraryId, userId) VALUES ('play-banned', 'Banned Playlist', '', 'lib-1', 'user-exclude-banned')`)
	if err != nil {
		t.Fatalf("Failed to insert banned playlist: %v", err)
	}
	_, err = db.Exec(`INSERT INTO playlistMediaItems (id, mediaItemId, mediaItemType, "order", playlistId) VALUES ('pmi-banned', 'item-banned', 'book', 1, 'play-banned')`)
	if err != nil {
		t.Fatalf("Failed to insert banned playlist media item: %v", err)
	}

	handler := AuthMiddleware(db, "mysecret", ServeOPDS(db))

	// ----------------------------------------------------
	// 4. Test Cases for Explicit Content Restriction
	// ----------------------------------------------------
	t.Run("Explicit Content Restriction", func(t *testing.T) {
		endpoints := []struct {
			name string
			url  string
		}{
			{"All items", "/opds/v1.2/libraries/lib-1/all"},
			{"Recent items", "/opds/v1.2/libraries/lib-1/recent"},
			{"Search", "/opds/v1.2/libraries/lib-1/search?q=Explicit"},
			{"Author items", "/opds/v1.2/libraries/lib-1/authors/author-explicit"},
			{"Series items", "/opds/v1.2/libraries/lib-1/series/series-explicit"},
			{"Collection items", "/opds/v1.2/libraries/lib-1/collections/coll-explicit"},
			{"Playlist items", "/opds/v1.2/libraries/lib-1/playlists/play-explicit"},
		}

		for _, tc := range endpoints {
			req := httptest.NewRequest("GET", tc.url, nil)
			req.SetBasicAuth("no-explicit-user", "mypassword")
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("Expected status 200 from %s endpoint (%s), got %d", tc.name, tc.url, rr.Code)
			}
			body := rr.Body.String()
			if strings.Contains(body, "Explicit Title Book") {
				t.Errorf("SECURITY VULNERABILITY: Explicit book was exposed in %s endpoint for user-no-explicit!", tc.name)
			}
		}
	})

	// ----------------------------------------------------
	// 5. Test Cases for Tag Exclusion (SelectedTagsNotAccessible)
	// ----------------------------------------------------
	t.Run("Tag Exclusion Restriction", func(t *testing.T) {
		endpoints := []struct {
			name string
			url  string
		}{
			{"All items", "/opds/v1.2/libraries/lib-1/all"},
			{"Recent items", "/opds/v1.2/libraries/lib-1/recent"},
			{"Search", "/opds/v1.2/libraries/lib-1/search?q=Banned"},
			{"Author items", "/opds/v1.2/libraries/lib-1/authors/author-banned"},
			{"Series items", "/opds/v1.2/libraries/lib-1/series/series-banned"},
			{"Collection items", "/opds/v1.2/libraries/lib-1/collections/coll-banned"},
			{"Playlist items", "/opds/v1.2/libraries/lib-1/playlists/play-banned"},
		}

		for _, tc := range endpoints {
			req := httptest.NewRequest("GET", tc.url, nil)
			req.SetBasicAuth("exclude-banned-user", "mypassword")
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("Expected status 200 from %s endpoint (%s), got %d", tc.name, tc.url, rr.Code)
			}
			body := rr.Body.String()
			if strings.Contains(body, "Banned Title Book") {
				t.Errorf("SECURITY VULNERABILITY: Banned book was exposed in %s endpoint for user-exclude-banned!", tc.name)
			}
		}
	})
}
