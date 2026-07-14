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
