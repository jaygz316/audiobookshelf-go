package db

import (
	"database/sql"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"testing"
)

func TestGetServerSettings(t *testing.T) {
	// Setup a test database
	tmpDir, err := os.MkdirTemp("", "testdb-dir")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Test GetServerSettings with a valid, initialized DB
	settings, err := GetServerSettings(database)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if settings == nil {
		t.Fatalf("Expected settings to not be nil")
	}
	if settings.Language != "en-us" {
		t.Errorf("Expected settings with default language 'en-us', got: %s", settings.Language)
	}
	if len(settings.AuthActiveAuthMethods) != 1 || settings.AuthActiveAuthMethods[0] != "local" {
		t.Errorf("Expected AuthActiveAuthMethods to be ['local'], got: %v", settings.AuthActiveAuthMethods)
	}

	// Test GetServerSettings with nil database
	_, err = GetServerSettings(nil)
	if err == nil {
		t.Error("Expected error with nil database, got nil")
	} else if err.Error() != "database not initialized" {
		t.Errorf("Expected 'database not initialized' error, got: %v", err)
	}
}

func TestMigrateDatabase(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testdb-migrate-dir")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test_migrate.db")

	// 1. Create a legacy database manually
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}

	_, err = db.Exec("CREATE TABLE apiKeys (id TEXT PRIMARY KEY, isActive INTEGER, expiresAt TEXT, userId TEXT)")
	if err != nil {
		db.Close()
		t.Fatalf("Failed to create legacy table: %v", err)
	}
	db.Close()

	// 2. Open via InitDB, which should trigger migrateDatabase
	database, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed on existing legacy DB: %v", err)
	}
	defer database.Close()

	// 3. Verify columns name and createdAt exist now
	rows, err := database.Query("PRAGMA table_info(apiKeys)")
	if err != nil {
		t.Fatalf("Failed to query table_info: %v", err)
	}
	defer rows.Close()

	hasName := false
	hasCreatedAt := false
	for rows.Next() {
		var cid int
		var name string
		var typeStr string
		var notnull int
		var dfltValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltValue, &pk); err != nil {
			t.Fatalf("Failed to scan table_info row: %v", err)
		}
		if name == "name" {
			hasName = true
		}
		if name == "createdAt" {
			hasCreatedAt = true
		}
	}

	if !hasName {
		t.Errorf("Expected column 'name' to be added by migration")
	}
	if !hasCreatedAt {
		t.Errorf("Expected column 'createdAt' to be added by migration")
	}
}

func TestDBIndexes(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testdb-indexes-dir")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test_indexes.db")
	database, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()

	expectedIndexes := []string{
		"idx_libraryItems_libraryId",
		"idx_libraryItems_mediaId_mediaType",
		"idx_libraryItems_libraryFolderId",
		"idx_libraryFolders_libraryId",
		"idx_bookAuthors_bookId_authorId",
		"idx_bookAuthors_authorId_bookId",
		"idx_bookSeries_bookId_seriesId",
		"idx_bookSeries_seriesId_bookId",
		"idx_sessions_userId",
		"idx_mediaProgresses_userId_mediaItemId",
		"idx_playbackSessions_userId",
		"idx_playbackSessions_mediaItemId",
		"idx_podcastEpisodes_podcastId",
		"idx_playlists_userId",
		"idx_playlists_libraryId",
		"idx_playlistMediaItems_playlistId",
		"idx_playlistMediaItems_mediaItemId",
		"idx_collections_libraryId",
		"idx_collectionBooks_collectionId_bookId",
		"idx_collectionBooks_bookId_collectionId",
		"idx_customMetadataProviders_mediaType",
		"idx_authors_libraryId",
		"idx_shares_libraryItemId",
		"idx_feeds_userId",
		"idx_series_libraryId",
	}

	for _, idxName := range expectedIndexes {
		var count int
		err := database.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?", idxName).Scan(&count)
		if err != nil {
			t.Fatalf("Failed querying index %s: %v", idxName, err)
		}
		if count != 1 {
			t.Errorf("Expected index %s to exist, but it was not found", idxName)
		}
	}
}

func TestMigrationVersion7(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testdb-migrate7-dir")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test_migrate7.db")

	// 1. Create a database with schema version 6 manually
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}

	// Create necessary tables
	tables := []string{
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT, email TEXT, pash TEXT, type TEXT, token TEXT, isActive INTEGER, isLocked INTEGER, lastSeen INTEGER, permissions TEXT, bookmarks TEXT, extraData TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE sessions (id TEXT PRIMARY KEY, userId TEXT, ipAddress TEXT, userAgent TEXT, refreshToken TEXT, expiresAt TEXT, lastRefreshToken TEXT, lastRefreshTokenExpiresAt TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE apiKeys (id TEXT PRIMARY KEY, isActive INTEGER, expiresAt TEXT, userId TEXT, name TEXT, createdAt TEXT)`,
		`CREATE TABLE libraries (id TEXT PRIMARY KEY, name TEXT, displayOrder INTEGER, icon TEXT, mediaType TEXT, provider TEXT, lastScan TEXT, lastScanVersion TEXT, settings TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE libraryFolders (id TEXT PRIMARY KEY, path TEXT, libraryId TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE libraryItems (id TEXT PRIMARY KEY, ino TEXT, libraryId TEXT, path TEXT, relPath TEXT, isFile INTEGER, mtime TEXT, ctime TEXT, birthtime TEXT, createdAt TEXT, updatedAt TEXT, isMissing INTEGER, isInvalid INTEGER, mediaType TEXT, mediaId TEXT, size INTEGER, libraryFolderId TEXT, authorNamesFirstLast TEXT, authorNamesLastFirst TEXT, title TEXT, titleIgnorePrefix TEXT)`,
		`CREATE TABLE books (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT, subtitle TEXT, publishedYear TEXT, publishedDate TEXT, publisher TEXT, description TEXT, isbn TEXT, asin TEXT, language TEXT, explicit INTEGER, abridged INTEGER, coverPath TEXT, duration REAL, narrators BLOB, audioFiles BLOB, ebookFile BLOB, chapters BLOB, tags BLOB, genres BLOB, lockedFields BLOB)`,
		`CREATE TABLE podcasts (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT, author TEXT, releaseDate TEXT, feedURL TEXT, imageURL TEXT, description TEXT, itunesPageURL TEXT, itunesId TEXT, itunesArtistId TEXT, language TEXT, podcastType TEXT, explicit INTEGER, autoDownloadEpisodes INTEGER, autoDownloadSchedule TEXT, lastEpisodeCheck TEXT, maxEpisodesToKeep INTEGER, maxNewEpisodesToDownload INTEGER, coverPath TEXT, tags BLOB, genres BLOB, numEpisodes INTEGER, lockedFields BLOB)`,
		`CREATE TABLE bookSeries (bookId TEXT, seriesId TEXT, sequence TEXT)`,
		`CREATE TABLE series (id TEXT PRIMARY KEY, libraryId TEXT, name TEXT, nameIgnorePrefix TEXT, description TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE mediaProgresses (id TEXT PRIMARY KEY, userId TEXT, mediaItemId TEXT, mediaItemType TEXT, duration REAL, currentTime REAL, isFinished INTEGER, hideFromContinueListening INTEGER, ebookLocation TEXT, ebookProgress REAL, finishedAt TEXT, extraData TEXT, podcastId TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE playbackSessions (id TEXT PRIMARY KEY, userId TEXT, mediaItemId TEXT, mediaItemType TEXT, startTime REAL, libraryId TEXT, extraData TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE podcastEpisodes (id TEXT PRIMARY KEY, podcastId TEXT, title TEXT, audioFile TEXT)`,
		`CREATE TABLE playlists (id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT, createdAt TEXT, updatedAt TEXT, libraryId TEXT, userId TEXT)`,
		`CREATE TABLE playlistMediaItems (id TEXT PRIMARY KEY, mediaItemId TEXT, mediaItemType TEXT, "order" INTEGER, createdAt TEXT, playlistId TEXT)`,
		`CREATE TABLE collections (id TEXT PRIMARY KEY, libraryId TEXT, name TEXT, description TEXT, createdAt TEXT, updatedAt TEXT, isSmart INTEGER DEFAULT 0, rules TEXT)`,
		`CREATE TABLE collectionBooks (id TEXT PRIMARY KEY, "order" INTEGER, createdAt TEXT, bookId TEXT, collectionId TEXT)`,
		`CREATE TABLE customMetadataProviders (id TEXT PRIMARY KEY, name TEXT, mediaType TEXT, url TEXT, authHeaderValue TEXT, extraData TEXT, createdAt INTEGER, updatedAt INTEGER)`,
		`CREATE TABLE authors (id TEXT PRIMARY KEY, name TEXT, lastFirst TEXT, asin TEXT, description TEXT, imagePath TEXT, createdAt TEXT, updatedAt TEXT, libraryId TEXT)`,
		`CREATE TABLE bookAuthors (bookId TEXT, authorId TEXT)`,
		`CREATE TABLE shares (id TEXT PRIMARY KEY, libraryItemId TEXT, createdBy TEXT, expiresAt TEXT, isDownloadable INTEGER, pash TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE feeds (id TEXT PRIMARY KEY, type TEXT, entityId TEXT, userId TEXT, serverAddress TEXT, createdAt TEXT, updatedAt TEXT)`,
	}

	for _, tbl := range tables {
		if _, err := db.Exec(tbl); err != nil {
			db.Close()
			t.Fatalf("Failed to create table: %v", err)
		}
	}

	// Set user_version to 6
	_, err = db.Exec("PRAGMA user_version = 6")
	if err != nil {
		db.Close()
		t.Fatalf("Failed to set user_version: %v", err)
	}
	db.Close()

	// 2. Open via InitDB, which should trigger migrateDatabase and run version 7 migration
	database, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed on existing legacy DB: %v", err)
	}
	defer database.Close()

	// 3. Verify all 25 indexes exist
	expectedIndexes := []string{
		"idx_libraryItems_libraryId",
		"idx_libraryItems_mediaId_mediaType",
		"idx_libraryItems_libraryFolderId",
		"idx_libraryFolders_libraryId",
		"idx_bookAuthors_bookId_authorId",
		"idx_bookAuthors_authorId_bookId",
		"idx_bookSeries_bookId_seriesId",
		"idx_bookSeries_seriesId_bookId",
		"idx_sessions_userId",
		"idx_mediaProgresses_userId_mediaItemId",
		"idx_playbackSessions_userId",
		"idx_playbackSessions_mediaItemId",
		"idx_podcastEpisodes_podcastId",
		"idx_playlists_userId",
		"idx_playlists_libraryId",
		"idx_playlistMediaItems_playlistId",
		"idx_playlistMediaItems_mediaItemId",
		"idx_collections_libraryId",
		"idx_collectionBooks_collectionId_bookId",
		"idx_collectionBooks_bookId_collectionId",
		"idx_customMetadataProviders_mediaType",
		"idx_authors_libraryId",
		"idx_shares_libraryItemId",
		"idx_feeds_userId",
		"idx_series_libraryId",
	}

	for _, idxName := range expectedIndexes {
		var count int
		err := database.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?", idxName).Scan(&count)
		if err != nil {
			t.Fatalf("Failed querying index %s: %v", idxName, err)
		}
		if count != 1 {
			t.Errorf("Expected index %s to exist, but it was not found after migration", idxName)
		}
	}
}

func TestInitDBConnectionPooling(t *testing.T) {
	// Set environment overrides
	os.Setenv("DB_MAX_OPEN_CONNS", "15")
	os.Setenv("DB_MAX_IDLE_CONNS", "5")
	os.Setenv("DB_CONN_MAX_LIFETIME", "30m")
	os.Setenv("DB_CONN_MAX_IDLE_TIME", "10m")
	defer func() {
		os.Unsetenv("DB_MAX_OPEN_CONNS")
		os.Unsetenv("DB_MAX_IDLE_CONNS")
		os.Unsetenv("DB_CONN_MAX_LIFETIME")
		os.Unsetenv("DB_CONN_MAX_IDLE_TIME")
	}()

	tmpDir, err := os.MkdirTemp("", "testdb-pool-dir")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test_pool.db")
	database, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()

	stats := database.Stats()
	if stats.MaxOpenConnections != 15 {
		t.Errorf("Expected MaxOpenConnections to be 15, got %d", stats.MaxOpenConnections)
	}
}

func TestGetTokenSecret(t *testing.T) {
	// Setup test database
	tmpDir, err := os.MkdirTemp("", "testdb-token-dir")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test_token.db")
	database, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()

	// Backup environment variables
	origEnvSecret := os.Getenv("JWT_SECRET_KEY")
	os.Unsetenv("JWT_SECRET_KEY")
	defer func() {
		if origEnvSecret != "" {
			os.Setenv("JWT_SECRET_KEY", origEnvSecret)
		} else {
			os.Unsetenv("JWT_SECRET_KEY")
		}
	}()

	// 1. Verify a nil database returns empty string
	nilSecret := GetTokenSecret(nil)
	if nilSecret != "" {
		t.Errorf("Expected GetTokenSecret(nil) to be empty, got: %s", nilSecret)
	}

	// 2. Call GetTokenSecret on empty settings. It should generate and save a new secret.
	secret1 := GetTokenSecret(database)
	if len(secret1) != 64 { // 32 bytes encoded in hex
		t.Errorf("Expected 64-char hex secret, got length %d: %s", len(secret1), secret1)
	}

	// 3. Subsequent calls should return the same secret
	secret2 := GetTokenSecret(database)
	if secret1 != secret2 {
		t.Errorf("Expected subsequent calls to return same secret, got: %s vs %s", secret1, secret2)
	}

	// 4. Verify environment variable override
	os.Setenv("JWT_SECRET_KEY", "env-override-secret-value-12345")
	envSecret := GetTokenSecret(database)
	if envSecret != "env-override-secret-value-12345" {
		t.Errorf("Expected environment override to take precedence, got: %s", envSecret)
	}
}
