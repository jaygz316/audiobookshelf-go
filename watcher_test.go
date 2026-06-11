package main

import (
	"testing"
)

// TestShouldIgnorePathRoot is a thin test that validates the root-level wrapper
// delegates correctly to the internal/watcher.ShouldIgnorePath function.
// The detailed tests live in internal/watcher/watcher_test.go.
func TestShouldIgnorePathRoot(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/path/to/.DS_Store", true},
		{"/path/to/normal.mp3", false},
		{"/path/to/downloading.tmp", true},
	}

	for _, tt := range tests {
		got := shouldIgnorePath(tt.path)
		if got != tt.want {
			t.Errorf("shouldIgnorePath(%q) = %t; want %t", tt.path, got, tt.want)
		}
	}
}
