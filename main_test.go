package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"audiobookshelf/internal/core")

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open memory db: %v", err)
	}

	// Create tables
	queries := []string{
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT)`,
		`CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT, type TEXT, isActive INTEGER, permissions TEXT, extraData TEXT)`,
		`CREATE TABLE apiKeys (id TEXT PRIMARY KEY, isActive INTEGER, expiresAt TEXT, userId TEXT)`,
		`CREATE TABLE libraries (id TEXT PRIMARY KEY, name TEXT, displayOrder INTEGER, icon TEXT, mediaType TEXT, provider TEXT, lastScan TEXT, lastScanVersion TEXT, settings TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE libraryFolders (id TEXT PRIMARY KEY, path TEXT, libraryId TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE libraryItems (id TEXT PRIMARY KEY, ino TEXT, libraryId TEXT, path TEXT, relPath TEXT, isFile INTEGER, mtime TEXT, ctime TEXT, birthtime TEXT, createdAt TEXT, updatedAt TEXT, isMissing INTEGER, isInvalid INTEGER, mediaType TEXT, mediaId TEXT, size INTEGER, libraryFolderId TEXT, authorNamesFirstLast TEXT, authorNamesLastFirst TEXT, title TEXT, titleIgnorePrefix TEXT)`,
		`CREATE TABLE books (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT, subtitle TEXT, publishedYear TEXT, publishedDate TEXT, publisher TEXT, description TEXT, isbn TEXT, asin TEXT, language TEXT, explicit INTEGER, abridged INTEGER, coverPath TEXT, duration REAL, narrators BLOB, audioFiles BLOB, ebookFile BLOB, chapters BLOB, tags BLOB, genres BLOB)`,
		`CREATE TABLE podcasts (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT, author TEXT, releaseDate TEXT, feedURL TEXT, imageURL TEXT, description TEXT, itunesPageURL TEXT, itunesId TEXT, itunesArtistId TEXT, language TEXT, podcastType TEXT, explicit INTEGER, autoDownloadEpisodes INTEGER, autoDownloadSchedule TEXT, lastEpisodeCheck TEXT, maxEpisodesToKeep INTEGER, maxNewEpisodesToDownload INTEGER, coverPath TEXT, tags BLOB, genres BLOB, numEpisodes INTEGER)`,
		`CREATE TABLE bookSeries (bookId TEXT, seriesId TEXT, sequence TEXT)`,
		`CREATE TABLE series (id TEXT PRIMARY KEY, name TEXT)`,
		`CREATE TABLE mediaProgresses (id TEXT PRIMARY KEY, userId TEXT, mediaItemId TEXT, isFinished INTEGER, currentTime REAL, updatedAt TEXT)`,
		`CREATE TABLE playbackSessions (id TEXT PRIMARY KEY, userId TEXT, mediaItemId TEXT, mediaItemType TEXT, startTime REAL, libraryId TEXT, extraData TEXT)`,
		`CREATE TABLE podcastEpisodes (id TEXT PRIMARY KEY, podcastId TEXT, title TEXT, audioFile TEXT)`,
		`CREATE TABLE playlists (id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT, createdAt TEXT, updatedAt TEXT, libraryId TEXT, userId TEXT)`,
		`CREATE TABLE playlistMediaItems (id TEXT PRIMARY KEY, mediaItemId TEXT, mediaItemType TEXT, "order" INTEGER, createdAt TEXT, playlistId TEXT)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("Failed to execute query %q: %v", q, err)
		}
	}

	// Insert settings
	_, err = db.Exec(`INSERT INTO settings (key, value) VALUES ('server-settings', '{"sortingIgnorePrefix": true}')`)
	if err != nil {
		t.Fatalf("Failed to insert settings: %v", err)
	}

	return db
}

func TestGetLibraries(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert a library
	_, err := db.Exec(`INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, settings, createdAt, updatedAt) VALUES ('lib1', 'Audiobooks', 1, 'book', 'book', 'local', '{}', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}

	// Insert library folder
	_, err = db.Exec(`INSERT INTO libraryFolders (id, path, libraryId, createdAt) VALUES ('folder1', '/audiobooks', 'lib1', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert folder: %v", err)
	}

	handler := handleGetLibraries(db)
	req := httptest.NewRequest("GET", "/api/libraries", nil)

	// Inject admin user
	user := &core.UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
		AccessAllTags:      true,
	}
	ctx := context.WithValue(req.Context(), core.UserContextKey, user)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	var resp map[string][]*LibraryJSON
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	libs, ok := resp["libraries"]
	if !ok || len(libs) != 1 {
		t.Errorf("Expected 1 library, got response: %v", resp)
	}

	if libs[0].ID != "lib1" || libs[0].Name != "Audiobooks" {
		t.Errorf("Unexpected library content: %v", libs[0])
	}

	if len(libs[0].Folders) != 1 || libs[0].Folders[0].FullPath != "/audiobooks" {
		t.Errorf("Unexpected library folders: %v", libs[0].Folders)
	}
}

func TestGetLibraryByID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert library
	_, err := db.Exec(`INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, settings, createdAt, updatedAt) VALUES ('lib1', 'Audiobooks', 1, 'book', 'book', 'local', '{}', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}

	user := &core.UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
		AccessAllTags:      true,
	}

	// Test case 1: Standard GET without filterdata
	{
		handler := handleGetLibraryByID(db, "lib1")
		req := httptest.NewRequest("GET", "/api/libraries/lib1", nil)
		ctx := context.WithValue(req.Context(), core.UserContextKey, user)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rr.Code)
		}

		var lib LibraryJSON
		if err := json.Unmarshal(rr.Body.Bytes(), &lib); err != nil {
			t.Fatalf("Failed to parse JSON response: %v", err)
		}

		if lib.ID != "lib1" {
			t.Errorf("Expected library ID lib1, got %s", lib.ID)
		}
	}

	// Test case 2: GET with include=filterdata
	{
		// Insert a test playlist
		_, err := db.Exec(`INSERT INTO playlists (id, name, libraryId, userId, createdAt, updatedAt) VALUES ('playlist1', 'My Playlist', 'lib1', 'user1', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
		if err != nil {
			t.Fatalf("Failed to insert playlist: %v", err)
		}

		handler := handleGetLibraryByID(db, "lib1")
		req := httptest.NewRequest("GET", "/api/libraries/lib1?include=filterdata", nil)
		ctx := context.WithValue(req.Context(), core.UserContextKey, user)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse JSON response: %v", err)
		}

		// Verify wrapper keys
		if _, ok := resp["library"]; !ok {
			t.Errorf("Expected response to contain 'library' key")
		}
		if _, ok := resp["filterdata"]; !ok {
			t.Errorf("Expected response to contain 'filterdata' key")
		}
		if _, ok := resp["issues"]; !ok {
			t.Errorf("Expected response to contain 'issues' key")
		}
		if numPlaylists, ok := resp["numUserPlaylists"].(float64); !ok || numPlaylists != 1 {
			t.Errorf("Expected 'numUserPlaylists' to be 1, got %v", resp["numUserPlaylists"])
		}

		// Verify library detail inside wrapper
		libMap, ok := resp["library"].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected 'library' to be a map")
		}
		if libMap["id"] != "lib1" {
			t.Errorf("Expected library ID lib1, got %v", libMap["id"])
		}
	}
}

func TestGetLibraryItems(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert library
	_, err := db.Exec(`INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, settings, createdAt, updatedAt) VALUES ('lib1', 'Audiobooks', 1, 'book', 'book', 'local', '{}', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}

	// Insert book
	_, err = db.Exec(`INSERT INTO books (id, title, titleIgnorePrefix, explicit, abridged, coverPath, duration, narrators, audioFiles, ebookFile, chapters, tags, genres) VALUES ('book1', 'The Great Book', 'Great Book', 0, 0, 'covers/book1.jpg', 3600.0, '["Narrator A"]', '[]', 'null', '[]', '["TagA"]', '["GenreA"]')`)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	// Insert library item
	_, err = db.Exec(`INSERT INTO libraryItems (id, ino, libraryId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size, libraryFolderId, authorNamesFirstLast, authorNamesLastFirst, title, titleIgnorePrefix) VALUES ('item1', '12345', 'lib1', '/audiobooks/book1', 'book1', 0, '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', 0, 0, 'book', 'book1', 5000, 'folder1', 'Author X', 'Author, X', 'The Great Book', 'Great Book')`)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}

	handler := handleGetLibraryItems(db, "lib1")
	req := httptest.NewRequest("GET", "/api/libraries/lib1/items?sort=media.metadata.title&desc=0&minified=1", nil)

	user := &core.UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
		AccessAllTags:      true,
	}
	ctx := context.WithValue(req.Context(), core.UserContextKey, user)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	results, ok := resp["results"].([]interface{})
	if !ok || len(results) != 1 {
		t.Fatalf("Expected 1 result in results list, got response: %v", resp)
	}

	item := results[0].(map[string]interface{})
	if item["id"] != "item1" || item["mediaType"] != "book" {
		t.Errorf("Unexpected item values: %v", item)
	}

	media := item["media"].(map[string]interface{})
	metadata := media["metadata"].(map[string]interface{})
	if metadata["title"] != "The Great Book" || metadata["authorName"] != "Author X" {
		t.Errorf("Unexpected media metadata: %v", metadata)
	}
}

func TestCreateLibrary(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	payload := CreateLibraryPayload{
		Name:      "New Books Library",
		MediaType: "book",
		Folders: []CreateFolderPayload{
			{Path: t.TempDir()},
		},
	}
	body, _ := json.Marshal(payload)

	handler := handleCreateLibrary(db)
	req := httptest.NewRequest("POST", "/api/libraries", bytes.NewBuffer(body))

	user := &core.UserSession{
		ID:       "user1",
		Username: "admin",
		Type:     "admin",
		IsActive: true,
	}
	ctx := context.WithValue(req.Context(), core.UserContextKey, user)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var lib LibraryJSON
	if err := json.Unmarshal(rr.Body.Bytes(), &lib); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if lib.Name != "New Books Library" {
		t.Errorf("Expected library name New Books Library, got %s", lib.Name)
	}

	if len(lib.Folders) != 1 {
		t.Errorf("Expected 1 folder, got %d", len(lib.Folders))
	}
}

func TestUpdateLibrary(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert library
	_, err := db.Exec(`INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, settings, createdAt, updatedAt) VALUES ('lib1', 'Audiobooks', 1, 'book', 'book', 'local', '{}', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}

	newName := "Updated Audiobooks"
	payload := UpdateLibraryPayload{
		Name: &newName,
	}
	body, _ := json.Marshal(payload)

	handler := handleUpdateLibrary(db, "lib1")
	req := httptest.NewRequest("PATCH", "/api/libraries/lib1", bytes.NewBuffer(body))

	user := &core.UserSession{
		ID:       "user1",
		Username: "admin",
		Type:     "admin",
		IsActive: true,
	}
	ctx := context.WithValue(req.Context(), core.UserContextKey, user)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var lib LibraryJSON
	if err := json.Unmarshal(rr.Body.Bytes(), &lib); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if lib.Name != "Updated Audiobooks" {
		t.Errorf("Expected library name Updated Audiobooks, got %s", lib.Name)
	}
}

func TestDeleteLibrary(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert library
	_, err := db.Exec(`INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, settings, createdAt, updatedAt) VALUES ('lib1', 'Audiobooks', 1, 'book', 'book', 'local', '{}', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}

	// Insert book
	_, err = db.Exec(`INSERT INTO books (id, title, titleIgnorePrefix, explicit, abridged, coverPath, duration, narrators, audioFiles, ebookFile, chapters, tags, genres) VALUES ('book1', 'The Great Book', 'Great Book', 0, 0, 'covers/book1.jpg', 3600.0, '["Narrator A"]', '[]', 'null', '[]', '["TagA"]', '["GenreA"]')`)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	// Insert library item
	_, err = db.Exec(`INSERT INTO libraryItems (id, ino, libraryId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size, libraryFolderId, authorNamesFirstLast, authorNamesLastFirst, title, titleIgnorePrefix) VALUES ('item1', '12345', 'lib1', '/audiobooks/book1', 'book1', 0, '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', 0, 0, 'book', 'book1', 5000, 'folder1', 'Author X', 'Author, X', 'The Great Book', 'Great Book')`)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}

	handler := handleDeleteLibrary(db, "lib1")
	req := httptest.NewRequest("DELETE", "/api/libraries/lib1", nil)

	user := &core.UserSession{
		ID:       "user1",
		Username: "admin",
		Type:     "admin",
		IsActive: true,
	}
	ctx := context.WithValue(req.Context(), core.UserContextKey, user)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	// Ensure library is deleted
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM libraries WHERE id = 'lib1'").Scan(&count)
	if err != nil {
		t.Fatalf("Query libraries failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected library to be deleted")
	}

	// Ensure library item is deleted
	err = db.QueryRow("SELECT COUNT(*) FROM libraryItems WHERE id = 'item1'").Scan(&count)
	if err != nil {
		t.Fatalf("Query libraryItems failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected library item to be deleted")
	}

	// Ensure orphaned book is deleted
	err = db.QueryRow("SELECT COUNT(*) FROM books WHERE id = 'book1'").Scan(&count)
	if err != nil {
		t.Fatalf("Query books failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected orphaned book to be deleted")
	}
}

func TestHLSServing(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	globalDB = db

	tempDir := t.TempDir()
	sm := NewStreamManager()

	streamID := "test-session-123"
	s := &Stream{
		ID:                streamID,
		UserID:            "user1",
		LibraryItemID:     "item1",
		SegmentLength:     6.0,
		StreamPath:        tempDir,
		ConcatFilesPath:   filepath.Join(tempDir, "files.txt"),
		PlaylistPath:      filepath.Join(tempDir, "output.m3u8"),
		FinalPlaylistPath: filepath.Join(tempDir, "final-output.m3u8"),
		Tracks:            []Track{{Index: 0, Duration: 60.0, Path: "dummy.mp3"}},
		SegmentsCreated:   make(map[int]bool),
	}

	dummyPlaylist := getPlaylistStr("output", 60.0, 6.0, "mpegts")
	err := os.WriteFile(s.PlaylistPath, []byte(dummyPlaylist), 0644)
	if err != nil {
		t.Fatalf("Failed to write dummy playlist: %v", err)
	}

	dummySegPath := filepath.Join(tempDir, "output-0.ts")
	err = os.WriteFile(dummySegPath, []byte("dummy binary ts content"), 0644)
	if err != nil {
		t.Fatalf("Failed to write dummy segment: %v", err)
	}

	sm.AddStream(s)

	user := &core.UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
		AccessAllTags:      true,
	}

	req := httptest.NewRequest("GET", "/hls/"+streamID+"/output.m3u8", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user))
	rr := httptest.NewRecorder()

	handler := serveHLS(t.TempDir(), sm)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200 for playlist, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "#EXTM3U") {
		t.Errorf("Response body does not contain M3U8 tags: %s", rr.Body.String())
	}

	req = httptest.NewRequest("GET", "/hls/"+streamID+"/output-0.ts", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200 for segment 0, got %d", rr.Code)
	}
	if rr.Body.String() != "dummy binary ts content" {
		t.Errorf("Expected dummy segment content, got %q", rr.Body.String())
	}

	sm.RemoveStream(streamID)
}

func TestLoadOrCreateStream(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	globalDB = db

	_, err := db.Exec(`INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, extraData) 
		VALUES ('session-1', 'user-1', 'book-1', 'book', 12.0, '{"libraryItemId":"item-1"}')`)
	if err != nil {
		t.Fatalf("Failed to insert playback session: %v", err)
	}

	audioFilesJSON := `[
		{"index":0, "exclude":false, "duration":100.0, "codec":"mp3", "mimeType":"audio/mpeg", "metadata":{"path":"/fake/path.mp3"}}
	]`
	_, err = db.Exec(`INSERT INTO books (id, title, audioFiles) VALUES ('book-1', 'Fake Book', ?)`, audioFilesJSON)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	sm := NewStreamManager()
	tempDir := t.TempDir()

	s, err := sm.LoadOrCreateStream(db, "session-1", tempDir, nil)
	if err != nil {
		if !strings.Contains(err.Error(), "exec") && !strings.Contains(err.Error(), "failed to start transcode") {
			t.Errorf("Unexpected error from LoadOrCreateStream: %v", err)
		}
	} else {
		if s.ID != "session-1" {
			t.Errorf("Expected stream ID session-1, got %s", s.ID)
		}
		if len(s.Tracks) != 1 || s.Tracks[0].Path != "/fake/path.mp3" {
			t.Errorf("Unexpected tracks: %v", s.Tracks)
		}
		sm.RemoveStream("session-1")
	}
}

func TestGetLibraryPersonalized(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert library
	_, err := db.Exec(`INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, settings, createdAt, updatedAt) VALUES ('lib1', 'Audiobooks', 1, 'book', 'book', 'local', '{}', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}

	// Insert book
	_, err = db.Exec(`INSERT INTO books (id, title, titleIgnorePrefix, explicit, abridged, coverPath, duration, narrators, audioFiles, ebookFile, chapters, tags, genres) VALUES ('book1', 'The Great Book', 'Great Book', 0, 0, 'covers/book1.jpg', 3600.0, '["Narrator A"]', '[]', 'null', '[]', '["TagA"]', '["GenreA"]')`)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	// Insert library item
	_, err = db.Exec(`INSERT INTO libraryItems (id, ino, libraryId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size, libraryFolderId, authorNamesFirstLast, authorNamesLastFirst, title, titleIgnorePrefix) VALUES ('item1', '12345', 'lib1', '/audiobooks/book1', 'book1', 0, '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', 0, 0, 'book', 'book1', 5000, 'folder1', 'Author X', 'Author, X', 'The Great Book', 'Great Book')`)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}

	handler := handleGetLibraryPersonalized(db, "lib1")
	req := httptest.NewRequest("GET", "/api/libraries/lib1/personalized?limit=10", nil)

	user := &core.UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
		AccessAllTags:      true,
	}
	ctx := context.WithValue(req.Context(), core.UserContextKey, user)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var shelves []map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &shelves); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if len(shelves) != 1 {
		t.Fatalf("Expected 1 shelf (recently-added), got %d: %v", len(shelves), shelves)
	}

	shelf := shelves[0]
	if shelf["id"] != "recently-added" {
		t.Errorf("Expected recently-added shelf, got id: %v", shelf["id"])
	}

	entities := shelf["entities"].([]interface{})
	if len(entities) != 1 {
		t.Fatalf("Expected 1 entity in recently-added shelf, got %d", len(entities))
	}

	entity := entities[0].(map[string]interface{})
	if entity["id"] != "item1" {
		t.Errorf("Expected entity item1, got %v", entity["id"])
	}
}

func TestPlayItemRoute(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	globalDB = db

	// Insert library
	_, err := db.Exec(`INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, settings, createdAt, updatedAt) VALUES ('lib1', 'Audiobooks', 1, 'book', 'book', 'local', '{}', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}

	// Insert book with valid audioFiles
	audioFilesJSON := `[
		{"index":0, "exclude":false, "duration":100.0, "codec":"mp3", "mimeType":"audio/mpeg", "metadata":{"path":"/fake/path.mp3"}}
	]`
	_, err = db.Exec(`INSERT INTO books (id, title, audioFiles, duration) VALUES ('book1', 'The Great Book', ?, 100.0)`, audioFilesJSON)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	// Insert library item
	_, err = db.Exec(`INSERT INTO libraryItems (id, ino, libraryId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size, libraryFolderId, authorNamesFirstLast, authorNamesLastFirst, title, titleIgnorePrefix) VALUES ('item1', '12345', 'lib1', '/audiobooks/book1', 'book1', 0, '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', 0, 0, 'book', 'book1', 5000, 'folder1', 'Author X', 'Author, X', 'The Great Book', 'Great Book')`)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}

	// Setup Config
	cfg := &Config{
		RouterBasePath: "/audiobookshelf",
		MetadataPath:   t.TempDir(),
	}

	user := &core.UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
		AccessAllTags:      true,
	}

	handler := handleItemsDispatch(db, cfg)

	bodyBytes := []byte(`{"startTime": 0.0}`)
	req := httptest.NewRequest("POST", "/audiobookshelf/api/items/item1/play", bytes.NewBuffer(bodyBytes))
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code == http.StatusNotFound {
		t.Errorf("Expected route to be found, but got 404. Body: %s", rr.Body.String())
	}
}

