package utils

import (
	"database/sql"
	"path/filepath"
	"strings"
)

// IsSameOrSubPath checks if childPath is the same as or nested under parentPath.
func IsSameOrSubPath(parentPath, childPath string) bool {
	parentPath = filepath.Clean(parentPath)
	childPath = filepath.Clean(childPath)
	if parentPath == childPath {
		return true
	}
	rel, err := filepath.Rel(parentPath, childPath)
	if err != nil {
		return false
	}
	if rel == "" || rel == "." {
		return true
	}
	return !strings.HasPrefix(rel, "..")
}

// IsSafeFilePath checks if targetPath is safe to serve.
// A path is safe if it resolves to a location inside either the metadataPath or any of the configured library folders.
func IsSafeFilePath(db *sql.DB, metadataPath string, targetPath string) bool {
	if targetPath == "" {
		return false
	}

	// Resolve symlinks if the target or its parents contain them.
	// We'll walk up the path to find the longest existing prefix, resolve symlinks on it,
	// and then append the remaining parts.
	resolvedTarget, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		// If EvalSymlinks failed (likely because path or some component doesn't exist),
		// we find the existing parent directory and resolve symlinks on it.
		dir := targetPath
		var suffix []string
		for {
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			suffix = append([]string{filepath.Base(dir)}, suffix...)
			dir = parent
			if resolved, err := filepath.EvalSymlinks(dir); err == nil {
				resolvedTarget = filepath.Join(append([]string{resolved}, suffix...)...)
				break
			}
		}
		if resolvedTarget == "" {
			// Fallback to absolute clean path if resolution failed completely
			absTarget, err := filepath.Abs(targetPath)
			if err != nil {
				return false
			}
			resolvedTarget = absTarget
		}
	} else {
		// EvalSymlinks succeeded, now make it absolute to be sure
		absTarget, err := filepath.Abs(resolvedTarget)
		if err == nil {
			resolvedTarget = absTarget
		}
	}

	// 1. Check if it's inside metadataPath
	if metadataPath != "" {
		absMetadata, err := filepath.EvalSymlinks(metadataPath)
		if err != nil {
			absMetadata, _ = filepath.Abs(metadataPath)
		} else {
			absMetadata, _ = filepath.Abs(absMetadata)
		}
		if absMetadata != "" && IsSameOrSubPath(absMetadata, resolvedTarget) {
			return true
		}
	}

	// 2. Check if it's inside any configured library folders
	if db != nil {
		rows, err := db.Query("SELECT path FROM libraryFolders")
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var folderPath string
				if err := rows.Scan(&folderPath); err == nil && folderPath != "" {
					absFolder, err := filepath.EvalSymlinks(folderPath)
					if err != nil {
						absFolder, _ = filepath.Abs(folderPath)
					} else {
						absFolder, _ = filepath.Abs(absFolder)
					}
					if absFolder != "" && IsSameOrSubPath(absFolder, resolvedTarget) {
						return true
					}
				}
			}
		}
	}

	return false
}
