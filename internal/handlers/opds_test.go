package handlers

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func prepareOPDSTestDB(t *testing.T) *sql.DB {
	db := setupTestDB(t)

	// Recreate users table with full schema
	_, _ = db.Exec("DROP TABLE users")
	_, err := db.Exec(`
		CREATE TABLE users (
			id TEXT PRIMARY KEY,
			username TEXT,
			email TEXT,
			pash TEXT,
			type TEXT,
			token TEXT,
			isActive INTEGER,
			isLocked INTEGER,
			lastSeen TEXT,
			permissions TEXT,
			bookmarks TEXT,
			extraData TEXT,
			createdAt TEXT,
			updatedAt TEXT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create full users table: %v", err)
	}

	// Insert root user
	hashed, err := bcrypt.GenerateFromPassword([]byte("mypassword"), 8)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO users (id, username, type, pash, isActive, permissions) 
		VALUES ('user-root', 'admin-user', 'root', ?, 1, '{"accessAllLibraries": true}')
	`, string(hashed))
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	// Insert a library
	_, err = db.Exec(`
		INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, lastScan, lastScanVersion, settings, createdAt, updatedAt) 
		VALUES ('lib-1', 'Audiobooks', 1, '', 'book', '', '2026-07-10T00:00:00Z', '', '{}', '2026-07-10T00:00:00Z', '2026-07-10T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}

	// Insert book items in library
	_, err = db.Exec(`
		INSERT INTO libraryItems (id, ino, libraryId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size)
		VALUES ('item-1', '123', 'lib-1', '/books/book1', 'book1', 0, '123', '123', '123', '123', '1710000000000', 0, 0, 'book', 'book-1', 500)
	`)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO books (id, title, subtitle, publishedYear, publisher, description, coverPath, narrators, audioFiles, ebookFile, chapters, tags, genres)
		VALUES ('book-1', 'Test Book', 'A Cool Test', '2025', 'Publisher X', 'A descriptive summary', '/covers/book1.jpg', '[]', '[]', '{"ebookFormat": "epub", "metadata": {"size": 200}}', '[]', '[]', '[]')
	`)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	return db
}

func TestOPDS_Unauthorized(t *testing.T) {
	db := prepareOPDSTestDB(t)
	defer db.Close()

	handler := AuthMiddleware(db, "mysecret", ServeOPDS(db))

	req := httptest.NewRequest("GET", "/opds", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rr.Code)
	}

	wwwAuth := rr.Header().Get("WWW-Authenticate")
	if !strings.Contains(wwwAuth, "Basic") {
		t.Errorf("Expected WWW-Authenticate header to contain Basic, got: %q", wwwAuth)
	}
}

func TestOPDS_AuthorizedRoot(t *testing.T) {
	db := prepareOPDSTestDB(t)
	defer db.Close()

	handler := AuthMiddleware(db, "mysecret", ServeOPDS(db))

	req := httptest.NewRequest("GET", "/opds", nil)
	req.SetBasicAuth("admin-user", "mypassword")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	if !strings.Contains(body, `Audiobookshelf Go Catalog`) {
		t.Errorf("Expected feed title in response body, got: %s", body)
	}

	if !strings.Contains(body, `Browse library: Audiobooks`) {
		t.Errorf("Expected library entry in response body, got: %s", body)
	}

	if !strings.Contains(body, `/opds/v1.2/libraries/lib-1`) {
		t.Errorf("Expected library link in response body, got: %s", body)
	}
}

func TestOPDS_LibraryDetails(t *testing.T) {
	db := prepareOPDSTestDB(t)
	defer db.Close()

	handler := AuthMiddleware(db, "mysecret", ServeOPDS(db))

	req := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1", nil)
	req.SetBasicAuth("admin-user", "mypassword")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	if !strings.Contains(body, `<title>Audiobooks</title>`) {
		t.Errorf("Expected library feed title, got: %s", body)
	}

	if !strings.Contains(body, `/opds/v1.2/libraries/lib-1/all`) {
		t.Errorf("Expected subsection All Items link, got: %s", body)
	}

	if !strings.Contains(body, `/opds/v1.2/libraries/lib-1/recent`) {
		t.Errorf("Expected subsection Recent link, got: %s", body)
	}

	if !strings.Contains(body, `/opds/v1.2/libraries/lib-1/search?q={searchTerms}`) {
		t.Errorf("Expected search template, got: %s", body)
	}
}

func TestOPDS_LibraryAllItems(t *testing.T) {
	db := prepareOPDSTestDB(t)
	defer db.Close()

	handler := AuthMiddleware(db, "mysecret", ServeOPDS(db))

	req := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1/all", nil)
	req.SetBasicAuth("admin-user", "mypassword")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	if !strings.Contains(body, `<title>Test Book - A Cool Test</title>`) {
		t.Errorf("Expected book title in item list, got: %s", body)
	}

	if !strings.Contains(body, `/api/items/item-1/download`) {
		t.Errorf("Expected download link in item list, got: %s", body)
	}

	if !strings.Contains(body, `type="application/epub+zip"`) {
		t.Errorf("Expected EPUB acquisition mimetype, got: %s", body)
	}

	if !strings.Contains(body, `/api/items/item-1/cover`) {
		t.Errorf("Expected cover link in item list, got: %s", body)
	}
}

func TestOPDS_Search(t *testing.T) {
	db := prepareOPDSTestDB(t)
	defer db.Close()

	handler := AuthMiddleware(db, "mysecret", ServeOPDS(db))

	// Match query
	req := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1/search?q=Test", nil)
	req.SetBasicAuth("admin-user", "mypassword")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	if !strings.Contains(body, `<title>Test Book - A Cool Test</title>`) {
		t.Errorf("Expected matching search result, got: %s", body)
	}

	// Non-matching query
	reqNoMatch := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1/search?q=Nonexistent", nil)
	reqNoMatch.SetBasicAuth("admin-user", "mypassword")
	rrNoMatch := httptest.NewRecorder()

	handler.ServeHTTP(rrNoMatch, reqNoMatch)

	if rrNoMatch.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rrNoMatch.Code)
	}

	bodyNoMatch := rrNoMatch.Body.String()
	if strings.Contains(bodyNoMatch, `<title>Test Book - A Cool Test</title>`) {
		t.Errorf("Expected zero search results, but got item in feed: %s", bodyNoMatch)
	}
}
