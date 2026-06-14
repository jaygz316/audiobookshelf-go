package db

import (
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
