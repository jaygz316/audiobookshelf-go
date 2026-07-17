package backup

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DBReconnectFunc is the function signature for reconnecting to the database after restore.
type DBReconnectFunc func(dbPath string) error

// OnBeforeRestore is an optional hook executed before replacing the database file.
// Used to close active database connections to avoid locks and state corruption.
var OnBeforeRestore func()

// ApplyBackup performs low-level backup restore operations on db and metadata directories.
func ApplyBackup(ctx context.Context, configPath, metadataPath, backupDir, id string, reconnect DBReconnectFunc) error {
	if !BackupRestoreMu.TryLock() {
		return fmt.Errorf("backup or restore already in progress")
	}
	defer BackupRestoreMu.Unlock()

	if OnBeforeRestore != nil {
		OnBeforeRestore()
	}

	fullPath := filepath.Join(backupDir, id+".audiobookshelf")

	if err := restoreDBFile(configPath, fullPath); err != nil {
		if reconnect != nil {
			reconnect(filepath.Join(configPath, "absdatabase.sqlite"))
		}
		return err
	}

	if err := restoreMetadataFiles(metadataPath, fullPath); err != nil {
		if reconnect != nil {
			reconnect(filepath.Join(configPath, "absdatabase.sqlite"))
		}
		return err
	}

	if reconnect != nil {
		return reconnect(filepath.Join(configPath, "absdatabase.sqlite"))
	}
	return nil
}

func restoreDBFile(configPath, zipPath string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	var sqliteEntry *zip.File
	for _, f := range zr.File {
		if f.Name == "absdatabase.sqlite" {
			sqliteEntry = f
			break
		}
	}

	if sqliteEntry == nil {
		return fmt.Errorf("backup ZIP missing database file")
	}

	tempDBPath := filepath.Join(configPath, fmt.Sprintf("absdatabase-temp-%d.sqlite", time.Now().UnixNano()))
	tempFile, err := os.Create(tempDBPath)
	if err != nil {
		return err
	}
	defer tempFile.Close()

	rc, err := sqliteEntry.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	if _, err = io.Copy(tempFile, rc); err != nil {
		os.Remove(tempDBPath)
		return err
	}

	tempFile.Close()
	rc.Close()

	realDBPath := filepath.Join(configPath, "absdatabase.sqlite")

	// Close/remove stale WAL and SHM files
	_ = os.Remove(realDBPath + "-wal")
	_ = os.Remove(realDBPath + "-shm")

	if err := os.Remove(realDBPath); err != nil && !os.IsNotExist(err) {
		os.Remove(tempDBPath)
		return err
	}

	if err := os.Rename(tempDBPath, realDBPath); err != nil {
		os.Remove(tempDBPath)
		return err
	}

	return nil
}

func restoreMetadataFiles(metadataPath, zipPath string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	itemsDir := filepath.Join(metadataPath, "items")
	authorsDir := filepath.Join(metadataPath, "authors")

	for _, f := range zr.File {
		var baseDir string
		var rel string

		if strings.HasPrefix(f.Name, "metadata-items/") {
			baseDir = itemsDir
			rel = strings.TrimPrefix(f.Name, "metadata-items/")
		} else if strings.HasPrefix(f.Name, "metadata-authors/") {
			baseDir = authorsDir
			rel = strings.TrimPrefix(f.Name, "metadata-authors/")
		} else {
			continue
		}

		if rel == "" {
			continue
		}

		dest := filepath.Join(baseDir, rel)

		// Zip Slip vulnerability check
		if !isSafePath(baseDir, dest) {
			return fmt.Errorf("zip slip detected: path %s goes outside %s", dest, baseDir)
		}

		if err := extractZipEntry(f, dest); err != nil {
			return err
		}
	}
	return nil
}

func extractZipEntry(f *zip.File, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	frc, err := f.Open()
	if err != nil {
		return err
	}
	defer frc.Close()

	_, err = io.Copy(out, frc)
	return err
}
