package backup

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupIDTraversalValidation(t *testing.T) {
	tests := []struct {
		id    string
		valid bool
	}{
		{"2006-01-02T1504", true},
		{"valid-id-123", true},
		{"..", false},
		{"../etc/passwd", false},
		{"sub/dir", false},
		{"sub\\dir", false},
		{"id/../file", false},
		{"id\\..\\file", false},
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			if isValidBackupID(tc.id) != tc.valid {
				t.Errorf("isValidBackupID(%q) expected %v, got %v", tc.id, tc.valid, !tc.valid)
			}
		})
	}
}

func TestUploadFilenameSanitization(t *testing.T) {
	filenames := []struct {
		input    string
		expected string
	}{
		{"backup.audiobookshelf", "backup.audiobookshelf"},
		{"../../evil.audiobookshelf", "evil.audiobookshelf"},
		{"path/to/backup.audiobookshelf", "backup.audiobookshelf"},
		{"C:\\path\\to\\backup.audiobookshelf", "backup.audiobookshelf"},
	}

	for _, tc := range filenames {
		t.Run(tc.input, func(t *testing.T) {
			safeName := filepath.Base(strings.ReplaceAll(tc.input, "\\", "/"))
			if safeName != tc.expected {
				t.Errorf("filepath.Base(%q) expected %q, got %q", tc.input, tc.expected, safeName)
			}
		})
	}
}
