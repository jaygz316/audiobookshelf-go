package db

import (
	"database/sql"
	"fmt"
	"testing"

	_ "modernc.org/sqlite"
)

func setupAuthorsSeriesTestDB(t *testing.T) *sql.DB {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS authors (
			id TEXT PRIMARY KEY,
			imagePath TEXT
		);
		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT
		);
	`)
	if err != nil {
		t.Fatalf("Failed to create tables: %v", err)
	}

	return db
}

func TestGetAuthorImagePath(t *testing.T) {
	db := setupAuthorsSeriesTestDB(t)
	defer db.Close()

	// 1. Author not found
	_, err := GetAuthorImagePath(db, "nonexistent")
	if err == nil {
		t.Errorf("Expected error for nonexistent author, got nil")
	}

	// 2. Author with NULL imagePath
	_, err = db.Exec(`INSERT INTO authors (id, imagePath) VALUES ('author1', NULL)`)
	if err != nil {
		t.Fatalf("Failed to insert author1: %v", err)
	}
	path, err := GetAuthorImagePath(db, "author1")
	if err != nil {
		t.Errorf("Expected no error for author1, got: %v", err)
	}
	if path != "" {
		t.Errorf("Expected empty string for NULL imagePath, got: %v", path)
	}

	// 3. Author with valid imagePath
	_, err = db.Exec(`INSERT INTO authors (id, imagePath) VALUES ('author2', '/path/to/image.jpg')`)
	if err != nil {
		t.Fatalf("Failed to insert author2: %v", err)
	}
	path, err = GetAuthorImagePath(db, "author2")
	if err != nil {
		t.Errorf("Expected no error for author2, got: %v", err)
	}
	if path != "/path/to/image.jpg" {
		t.Errorf("Expected '/path/to/image.jpg', got: %v", path)
	}
}

func TestGetSortingPrefixes(t *testing.T) {
	db := setupAuthorsSeriesTestDB(t)
	defer db.Close()

	// 1. Key not found (should return defaults)
	prefixes := GetSortingPrefixes(db)
	if len(prefixes) != 2 || prefixes[0] != "the" || prefixes[1] != "a" {
		t.Errorf("Expected default prefixes ['the', 'a'], got: %v", prefixes)
	}

	// 2. Invalid JSON
	_, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('server-settings', 'invalid_json')`)
	if err != nil {
		t.Fatalf("Failed to insert invalid json: %v", err)
	}
	prefixes = GetSortingPrefixes(db)
	if len(prefixes) != 2 || prefixes[0] != "the" || prefixes[1] != "a" {
		t.Errorf("Expected default prefixes for invalid JSON, got: %v", prefixes)
	}

	// 3. Valid JSON
	_, err = db.Exec(`UPDATE settings SET value = '{"sortingPrefixes":["an", "the", "a"]}' WHERE key = 'server-settings'`)
	if err != nil {
		t.Fatalf("Failed to update valid json: %v", err)
	}
	prefixes = GetSortingPrefixes(db)
	if len(prefixes) != 3 || prefixes[0] != "an" || prefixes[1] != "the" || prefixes[2] != "a" {
		t.Errorf("Expected ['an', 'the', 'a'], got: %v", prefixes)
	}
}
