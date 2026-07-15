package handlers

import (
	"archive/zip"
	log "audiobookshelf/internal/logger"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/utils"
)

func streamDirAsZip(w io.Writer, dirPath string) error {
	zw := zip.NewWriter(w)
	defer zw.Close()

	return filepath.Walk(dirPath, func(path string, fileInfo os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if fileInfo.IsDir() {
			return nil
		}
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			log.Warnf("[Download] Skipping symlink in zip: %s", path)
			return nil
		}
		rel, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		header, err := zip.FileInfoHeader(fileInfo)
		if err != nil {
			return err
		}
		header.Name = rel
		header.Method = zip.Deflate

		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		_, err = io.Copy(writer, f)
		return err
	})
}

func serveDownload(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		if !user.CanDownload {
			log.Warnf("[Download] Forbidden: user %s does not have download permissions", user.Username)
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		parts := strings.Split(r.URL.Path, "/")
		var itemID string
		for i, part := range parts {
			if part == "items" && i+1 < len(parts) {
				itemID = parts[i+1]
				break
			}
		}

		if itemID == "" {
			http.Error(w, `{"error": "Invalid Item ID"}`, http.StatusBadRequest)
			return
		}

		info, err := idb.GetLibraryItemDownloadInfo(db, itemID)
		if err != nil {
			log.Errorf("[Download] Failed to get library item info: %v", err)
			http.Error(w, `{"error": "Library item not found"}`, http.StatusNotFound)
			return
		}

		if !utils.IsSafeFilePath(db, MetadataPath, info.Path) {
			log.Warnf("[Download] Path traversal blocked: %s", info.Path)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		log.Infof("[Download] user %s requested download for item %s (isFile: %t)", user.Username, itemID, info.IsFile)

		if info.IsFile {
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(info.RelPath)))
			http.ServeFile(w, r, info.Path)
			return
		}

		// Directory zip downloads: zip on-the-fly in Go
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q.zip", filepath.Base(info.Path)))
		if err := streamDirAsZip(w, info.Path); err != nil {
			log.Errorf("[Download] Directory zip failed: %v", err)
		}
	}
}
