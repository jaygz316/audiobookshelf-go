package handlers

import (
	log "audiobookshelf/internal/logger"
	"fmt"
	"os"
	"path/filepath"
)

// cleanAndCreateFolder cleans a folder path, converts separators, and ensures directory existence.
func cleanAndCreateFolder(fullPath, path string) (string, error) {
	fpath := fullPath
	if fpath == "" {
		fpath = path
	}
	if fpath == "" {
		return "", fmt.Errorf("Folder path is required")
	}
	absPath, err := filepath.Abs(fpath)
	if err != nil {
		absPath = fpath
	}
	absPath = filepath.ToSlash(absPath)
	if err := os.MkdirAll(absPath, 0755); err != nil {
		log.Errorf("Failed to create folder directory %s: %v", absPath, err)
		return "", fmt.Errorf("Invalid folder directory %s", absPath)
	}
	return absPath, nil
}
