package handlers

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/doyensec/safeurl"

	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/utils"
)

var coverHTTPClient *safeurl.WrappedClient

func init() {
	builder := safeurl.GetConfigBuilder()
	if os.Getenv("BYPASS_SAFEURL") == "true" {
		builder = builder.SetAllowedIPs("127.0.0.1", "::1")
		var ports []int
		for p := 1; p <= 65535; p++ {
			ports = append(ports, p)
		}
		builder = builder.SetAllowedPorts(ports...)
	}
	config := builder.Build()
	coverHTTPClient = safeurl.Client(config)
}

func getCoverFromCache(metadataPath, itemID, width, height, format string) (string, error) {
	cacheFilename := itemID + "_" + width
	if height != "" {
		cacheFilename += "x" + height
	}
	cacheFilename += "." + format
	cachePath := filepath.Join(metadataPath, "cache", "covers", cacheFilename)
	if _, err := os.Stat(cachePath); err != nil {
		return "", err
	}
	return cachePath, nil
}

func resizeImage(coverPath, cachePath, width, height, format string) error {
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	filter := fmt.Sprintf("scale=%s:-1", width)
	if height != "" {
		filter = fmt.Sprintf("scale=%s:%s", width, height)
	}

	args := []string{
		"-y",
		"-i", coverPath,
		"-vf", filter,
		cachePath,
	}

	cmd := exec.Command("ffmpeg", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg resize failed: %v, output: %s", err, string(output))
	}
	return nil
}

func determineCoverDestPath(db *sql.DB, metadataPath, itemID, mediaType, mediaID, itemPath string, isFile int, ext string) (string, error) {
	settings, err := idb.GetServerSettings(db)
	if err == nil && settings != nil && settings.MetadataCoverWithItem {
		folder := itemPath
		if isFile != 0 {
			folder = filepath.Dir(itemPath)
		}
		return filepath.Join(folder, "cover"+ext), nil
	}

	var existingCoverPath sql.NullString
	if mediaType == "book" {
		_ = db.QueryRow("SELECT coverPath FROM books WHERE id = ?", mediaID).Scan(&existingCoverPath)
	} else if mediaType == "podcast" {
		_ = db.QueryRow("SELECT coverPath FROM podcasts WHERE id = ?", mediaID).Scan(&existingCoverPath)
	}

	if existingCoverPath.Valid && existingCoverPath.String != "" {
		return existingCoverPath.String, nil
	}

	itemDir := filepath.Join(metadataPath, "items", itemID)
	if err := os.MkdirAll(itemDir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(itemDir, "cover"+ext), nil
}

func saveCoverFile(db *sql.DB, metadataPath, itemID, destPath, ext string, r io.Reader) (string, error) {
	if !utils.IsSafeFilePath(db, metadataPath, destPath) {
		return "", fmt.Errorf("forbidden: unsafe cover destination path")
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return "", err
	}

	out, err := os.Create(destPath)
	if err != nil {
		itemDir := filepath.Join(metadataPath, "items", itemID)
		destPath = filepath.Join(itemDir, "cover"+ext)
		if !utils.IsSafeFilePath(db, metadataPath, destPath) {
			return "", fmt.Errorf("forbidden: unsafe fallback cover destination path")
		}
		if err := os.MkdirAll(itemDir, 0755); err != nil {
			return "", err
		}
		out, err = os.Create(destPath)
		if err != nil {
			return "", err
		}
	}
	defer out.Close()

	if _, err = io.Copy(out, r); err != nil {
		return "", err
	}

	return filepath.ToSlash(destPath), nil
}

func updateCoverDatabaseAndClearCache(db *sql.DB, metadataPath, itemID, mediaType, mediaID, destPath string) error {
	var err error
	if mediaType == "book" {
		_, err = db.Exec("UPDATE books SET coverPath = ? WHERE id = ?", destPath, mediaID)
	} else if mediaType == "podcast" {
		_, err = db.Exec("UPDATE podcasts SET coverPath = ? WHERE id = ?", destPath, mediaID)
	}
	if err != nil {
		return err
	}

	nowStr := time.Now().Format("2006-01-02 15:04:05.000")
	_, _ = db.Exec("UPDATE libraryItems SET updatedAt = ? WHERE id = ?", nowStr, itemID)

	cachePattern := filepath.Join(metadataPath, "cache", "covers", itemID+"_*")
	if files, err := filepath.Glob(cachePattern); err == nil {
		for _, f := range files {
			_ = os.Remove(f)
		}
	}
	return nil
}
