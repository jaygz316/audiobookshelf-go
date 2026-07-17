package utils

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestIsSameOrSubPath(t *testing.T) {
	tests := []struct {
		name       string
		parentPath string
		childPath  string
		expected   bool
	}{
		{
			name:       "same path",
			parentPath: "/a/b",
			childPath:  "/a/b",
			expected:   true,
		},
		{
			name:       "child path nested",
			parentPath: "/a/b",
			childPath:  "/a/b/c",
			expected:   true,
		},
		{
			name:       "parent path not prefix",
			parentPath: "/a/b",
			childPath:  "/a/c",
			expected:   false,
		},
		{
			name:       "child path with dot dot traversal outside",
			parentPath: "/a/b",
			childPath:  "/a/b/../c",
			expected:   false,
		},
		{
			name:       "child path with dot dot traversal inside",
			parentPath: "/a/b",
			childPath:  "/a/b/c/../d",
			expected:   true,
		},
		{
			name:       "empty parent",
			parentPath: "",
			childPath:  "/a",
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSameOrSubPath(tt.parentPath, tt.childPath)
			if got != tt.expected {
				t.Errorf("IsSameOrSubPath(%q, %q) = %v, want %v", tt.parentPath, tt.childPath, got, tt.expected)
			}
		})
	}
}

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	_, err = db.Exec("CREATE TABLE libraryFolders (path TEXT)")
	if err != nil {
		db.Close()
		t.Fatalf("failed to create libraryFolders table: %v", err)
	}
	return db
}

func TestIsSafeFilePath(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tempDir := t.TempDir()
	metadataPath := filepath.Join(tempDir, "metadata")
	libraryPath := filepath.Join(tempDir, "library")
	privatePath := filepath.Join(tempDir, "private")

	err := os.MkdirAll(metadataPath, 0755)
	if err != nil {
		t.Fatalf("failed to create metadata dir: %v", err)
	}
	err = os.MkdirAll(libraryPath, 0755)
	if err != nil {
		t.Fatalf("failed to create library dir: %v", err)
	}
	err = os.MkdirAll(privatePath, 0755)
	if err != nil {
		t.Fatalf("failed to create private dir: %v", err)
	}

	// Insert safe library folder
	_, err = db.Exec("INSERT INTO libraryFolders (path) VALUES (?)", libraryPath)
	if err != nil {
		t.Fatalf("failed to insert library folder: %v", err)
	}

	tests := []struct {
		name         string
		metadataPath string
		targetPath   string
		expected     bool
	}{
		{
			name:         "empty target path",
			metadataPath: metadataPath,
			targetPath:   "",
			expected:     false,
		},
		{
			name:         "target inside metadata path",
			metadataPath: metadataPath,
			targetPath:   filepath.Join(metadataPath, "item1.jpg"),
			expected:     true,
		},
		{
			name:         "target inside library path",
			metadataPath: metadataPath,
			targetPath:   filepath.Join(libraryPath, "book1/audio.mp3"),
			expected:     true,
		},
		{
			name:         "target outside metadata and library paths",
			metadataPath: metadataPath,
			targetPath:   filepath.Join(privatePath, "secret.txt"),
			expected:     false,
		},
		{
			name:         "target traverses outside metadata path",
			metadataPath: metadataPath,
			targetPath:   filepath.Join(metadataPath, "../private/secret.txt"),
			expected:     false,
		},
		{
			name:         "target traverses outside library path",
			metadataPath: metadataPath,
			targetPath:   filepath.Join(libraryPath, "../private/secret.txt"),
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSafeFilePath(db, tt.metadataPath, tt.targetPath)
			if got != tt.expected {
				t.Errorf("IsSafeFilePath(db, %q, %q) = %v, want %v", tt.metadataPath, tt.targetPath, got, tt.expected)
			}
		})
	}
}

func TestIsSafeFilePath_Symlink(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "library")

	privateDir := t.TempDir()
	privatePath := filepath.Join(privateDir, "private")

	err := os.MkdirAll(libraryPath, 0755)
	if err != nil {
		t.Fatalf("failed to create library dir: %v", err)
	}
	err = os.MkdirAll(privatePath, 0755)
	if err != nil {
		t.Fatalf("failed to create private dir: %v", err)
	}

	secretFile := filepath.Join(privatePath, "secret.txt")
	err = os.WriteFile(secretFile, []byte("secret contents"), 0644)
	if err != nil {
		t.Fatalf("failed to write secret file: %v", err)
	}

	// Insert library folder path into db
	_, err = db.Exec("INSERT INTO libraryFolders (path) VALUES (?)", libraryPath)
	if err != nil {
		t.Fatalf("failed to insert library folder: %v", err)
	}

	// Create symlink inside library pointing to the secret file outside
	symlinkPath := filepath.Join(libraryPath, "secret_symlink.txt")
	err = os.Symlink(secretFile, symlinkPath)
	if err != nil {
		t.Skip("skipping symlink test; failed to create symlink: ", err)
		return
	}

	// Verify that IsSafeFilePath correctly rejects the symlink path
	got := IsSafeFilePath(db, tempDir, symlinkPath)
	if got {
		t.Error("SECURITY HOLE: IsSafeFilePath reported a symlink pointing to an outside file as safe!")
	}
}
