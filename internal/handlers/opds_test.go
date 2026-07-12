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

	// Insert book items in library with all fields populated to prevent sql Scan errors
	_, err = db.Exec(`
		INSERT INTO libraryItems (id, ino, libraryId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size, libraryFolderId, authorNamesFirstLast, authorNamesLastFirst, title, titleIgnorePrefix)
		VALUES ('item-1', '123', 'lib-1', '/books/book1', 'book1', 0, '123', '123', '123', '123', '1710000000000', 0, 0, 'book', 'book-1', 500, '', 'Author One', 'Author One', 'Test Book', 'Test Book')
	`)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO books (id, title, titleIgnorePrefix, subtitle, publishedYear, publishedDate, publisher, description, isbn, asin, language, explicit, abridged, coverPath, duration, narrators, audioFiles, ebookFile, chapters, tags, genres, lockedFields)
		VALUES ('book-1', 'Test Book', 'Test Book', 'A Cool Test', '2025', '2025', 'Publisher X', 'A descriptive summary', '', '', 'en', 0, 0, '/covers/book1.jpg', 0.0, '[]', '[]', '{"ebookFormat": "epub", "metadata": {"size": 200}}', '[]', '[]', '[]', '[]')
	`)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	// Setup authors table
	_, _ = db.Exec("DROP TABLE IF EXISTS authors")
	_, err = db.Exec(`
		CREATE TABLE authors (
			id TEXT PRIMARY KEY,
			name TEXT,
			description TEXT,
			libraryId TEXT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create authors table: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO authors (id, name, description, libraryId)
		VALUES ('author-1', 'Author One', 'Awesome Author', 'lib-1')
	`)
	if err != nil {
		t.Fatalf("Failed to insert author: %v", err)
	}

	_, _ = db.Exec("DROP TABLE IF EXISTS bookAuthors")
	_, err = db.Exec(`
		CREATE TABLE bookAuthors (
			bookId TEXT,
			authorId TEXT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create bookAuthors table: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO bookAuthors (bookId, authorId)
		VALUES ('book-1', 'author-1')
	`)
	if err != nil {
		t.Fatalf("Failed to insert book-author link: %v", err)
	}

	// Setup series table
	_, _ = db.Exec("DROP TABLE IF EXISTS series")
	_, err = db.Exec(`
		CREATE TABLE series (
			id TEXT PRIMARY KEY,
			name TEXT,
			description TEXT,
			libraryId TEXT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create series table: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO series (id, name, description, libraryId)
		VALUES ('series-1', 'Series One', 'Epic Series', 'lib-1')
	`)
	if err != nil {
		t.Fatalf("Failed to insert series: %v", err)
	}

	_, _ = db.Exec("DROP TABLE IF EXISTS bookSeries")
	_, err = db.Exec(`
		CREATE TABLE bookSeries (
			bookId TEXT,
			seriesId TEXT,
			sequence TEXT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create bookSeries table: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO bookSeries (bookId, seriesId, sequence)
		VALUES ('book-1', 'series-1', '1.0')
	`)
	if err != nil {
		t.Fatalf("Failed to insert book-series link: %v", err)
	}

	// Setup collections table
	_, _ = db.Exec("DROP TABLE IF EXISTS collections")
	_, err = db.Exec(`
		CREATE TABLE collections (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			libraryId TEXT,
			isSmart INTEGER DEFAULT 0,
			rules TEXT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create collections table: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO collections (id, name, description, libraryId)
		VALUES ('coll-1', 'Collection One', 'Super Collection', 'lib-1')
	`)
	if err != nil {
		t.Fatalf("Failed to insert collection: %v", err)
	}

	_, _ = db.Exec("DROP TABLE IF EXISTS collectionBooks")
	_, err = db.Exec(`
		CREATE TABLE collectionBooks (
			id TEXT PRIMARY KEY,
			"order" INTEGER,
			bookId TEXT,
			collectionId TEXT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create collectionBooks table: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO collectionBooks (id, "order", bookId, collectionId)
		VALUES ('cb-1', 1, 'book-1', 'coll-1')
	`)
	if err != nil {
		t.Fatalf("Failed to insert collection-book link: %v", err)
	}

	// Setup playlists and playlistMediaItems (relying on setupTestDB schemas)
	_, err = db.Exec(`
		INSERT INTO playlists (id, name, description, libraryId, userId)
		VALUES ('play-1', 'Playlist One', 'Chill Playlist', 'lib-1', 'user-root')
	`)
	if err != nil {
		t.Fatalf("Failed to insert playlist: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO playlistMediaItems (id, mediaItemId, mediaItemType, "order", playlistId)
		VALUES ('pmi-1', 'item-1', 'book', 1, 'play-1')
	`)
	if err != nil {
		t.Fatalf("Failed to insert playlist media item link: %v", err)
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

func TestOPDS_LibraryDetails_NewSubsections(t *testing.T) {
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
	subsections := []string{
		`/opds/v1.2/libraries/lib-1/authors`,
		`/opds/v1.2/libraries/lib-1/series`,
		`/opds/v1.2/libraries/lib-1/collections`,
		`/opds/v1.2/libraries/lib-1/playlists`,
	}
	for _, sub := range subsections {
		if !strings.Contains(body, sub) {
			t.Errorf("Expected library details to link to %s", sub)
		}
	}
}

func TestOPDS_Authors(t *testing.T) {
	db := prepareOPDSTestDB(t)
	defer db.Close()

	handler := AuthMiddleware(db, "mysecret", ServeOPDS(db))

	// Get authors list
	req := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1/authors", nil)
	req.SetBasicAuth("admin-user", "mypassword")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Author One") || !strings.Contains(body, "/opds/v1.2/libraries/lib-1/authors/author-1") {
		t.Errorf("Expected author feed to contain Author One and link, got: %s", body)
	}

	// Get items by author
	reqItems := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1/authors/author-1", nil)
	reqItems.SetBasicAuth("admin-user", "mypassword")
	rrItems := httptest.NewRecorder()
	handler.ServeHTTP(rrItems, reqItems)

	if rrItems.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rrItems.Code)
	}
	bodyItems := rrItems.Body.String()
	if !strings.Contains(bodyItems, "Test Book") {
		t.Errorf("Expected author items feed to contain Test Book, got: %s", bodyItems)
	}
}

func TestOPDS_Series(t *testing.T) {
	db := prepareOPDSTestDB(t)
	defer db.Close()

	handler := AuthMiddleware(db, "mysecret", ServeOPDS(db))

	// Get series list
	req := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1/series", nil)
	req.SetBasicAuth("admin-user", "mypassword")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Series One") || !strings.Contains(body, "/opds/v1.2/libraries/lib-1/series/series-1") {
		t.Errorf("Expected series feed to contain Series One and link, got: %s", body)
	}

	// Get items in series
	reqItems := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1/series/series-1", nil)
	reqItems.SetBasicAuth("admin-user", "mypassword")
	rrItems := httptest.NewRecorder()
	handler.ServeHTTP(rrItems, reqItems)

	if rrItems.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rrItems.Code)
	}
	bodyItems := rrItems.Body.String()
	if !strings.Contains(bodyItems, "Test Book") {
		t.Errorf("Expected series items feed to contain Test Book, got: %s", bodyItems)
	}
}

func TestOPDS_Collections(t *testing.T) {
	db := prepareOPDSTestDB(t)
	defer db.Close()

	handler := AuthMiddleware(db, "mysecret", ServeOPDS(db))

	// Get collections list
	req := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1/collections", nil)
	req.SetBasicAuth("admin-user", "mypassword")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Collection One") || !strings.Contains(body, "/opds/v1.2/libraries/lib-1/collections/coll-1") {
		t.Errorf("Expected collections feed to contain Collection One and link, got: %s", body)
	}

	// Get items in collection
	reqItems := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1/collections/coll-1", nil)
	reqItems.SetBasicAuth("admin-user", "mypassword")
	rrItems := httptest.NewRecorder()
	handler.ServeHTTP(rrItems, reqItems)

	if rrItems.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rrItems.Code)
	}
	bodyItems := rrItems.Body.String()
	if !strings.Contains(bodyItems, "Test Book") {
		t.Errorf("Expected collection items feed to contain Test Book, got: %s", bodyItems)
	}
}

func TestOPDS_Playlists(t *testing.T) {
	db := prepareOPDSTestDB(t)
	defer db.Close()

	handler := AuthMiddleware(db, "mysecret", ServeOPDS(db))

	// Get playlists list
	req := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1/playlists", nil)
	req.SetBasicAuth("admin-user", "mypassword")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Playlist One") || !strings.Contains(body, "/opds/v1.2/libraries/lib-1/playlists/play-1") {
		t.Errorf("Expected playlists feed to contain Playlist One and link, got: %s", body)
	}

	// Get items in playlist
	reqItems := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1/playlists/play-1", nil)
	reqItems.SetBasicAuth("admin-user", "mypassword")
	rrItems := httptest.NewRecorder()
	handler.ServeHTTP(rrItems, reqItems)

	if rrItems.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rrItems.Code)
	}
	bodyItems := rrItems.Body.String()
	if !strings.Contains(bodyItems, "Test Book") {
		t.Errorf("Expected playlist items feed to contain Test Book, got: %s", bodyItems)
	}
}
