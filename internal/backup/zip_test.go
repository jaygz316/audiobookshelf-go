package backup

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestZipSlipPrevention(t *testing.T) {
	baseDir := filepath.Join(os.TempDir(), "abs-test-base")

	tests := []struct {
		name       string
		targetPath string
		expected   bool
	}{
		{"Inside base dir", filepath.Join(baseDir, "items", "file.json"), true},
		{"Exactly base dir", baseDir, true},
		{"Direct parent", filepath.Join(baseDir, "..", "attacker.txt"), false},
		{"Relative sibling", filepath.Join(baseDir, "items", "..", "..", "attacker.txt"), false},
		{"Windows-style traversal", filepath.Join(baseDir, "items\\..\\..\\attacker.txt"), os.PathSeparator != '\\'},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isSafePath(baseDir, tc.targetPath)
			if result != tc.expected {
				t.Errorf("isSafePath(%q, %q) expected %v, got %v", baseDir, tc.targetPath, tc.expected, result)
			}
		})
	}
}

func TestZipSlipExtractionPrevention(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "abs-zipslip-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	zipPath := filepath.Join(tempDir, "malicious.zip")
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	// Add a Zip Slip entry: metadata-items/../../attacker.txt
	h := &zip.FileHeader{
		Name:   "metadata-items/../../attacker.txt",
		Method: zip.Store,
	}
	w, err := zw.CreateHeader(h)
	if err != nil {
		t.Fatalf("failed to create zip header: %v", err)
	}
	w.Write([]byte("malicious payload"))

	zw.Close()

	if err := os.WriteFile(zipPath, buf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write zip file: %v", err)
	}

	metadataPath := filepath.Join(tempDir, "metadata")
	err = restoreMetadataFiles(metadataPath, zipPath)
	if err == nil {
		t.Fatalf("expected restoreMetadataFiles to fail with zip slip error, but it succeeded")
	}

	if !strings.Contains(err.Error(), "zip slip detected") {
		t.Errorf("expected error message to contain 'zip slip detected', got %v", err)
	}

	// Verify that attacker.txt was not created outside the metadata items directory
	attackerPath := filepath.Join(metadataPath, "..", "attacker.txt")
	if _, err := os.Stat(attackerPath); !os.IsNotExist(err) {
		t.Errorf("Zip Slip vulnerability exploited! File written outside directory: %s", attackerPath)
	}
}
