package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/utils"
)

// handleGetPublicShare resolves GET /api/s/{slug}
func handleGetPublicShare(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		// Extract slug from URL path
		parts := strings.Split(r.URL.Path, "/")
		var slug string
		for i, part := range parts {
			if part == "s" && i+1 < len(parts) {
				slug = parts[i+1]
				break
			}
		}
		if slug == "" {
			http.Error(w, `{"error": "Invalid Slug"}`, http.StatusBadRequest)
			return
		}

		// Retrieve the share link
		s, err := globalShareManager.GetShare(r.Context(), slug)
		if err != nil {
			log.Errorf("[PublicShare] GetShare failed: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}
		if s == nil {
			http.Error(w, `{"error": "Share not found or expired"}`, http.StatusNotFound)
			return
		}

		// Check password protection
		hasPassword := s.PasswordHash != ""
		password := r.URL.Query().Get("password")
		if password == "" {
			password = r.Header.Get("X-Share-Password")
		}

		if hasPassword {
			valid, err := globalShareManager.ValidateSharePassword(r.Context(), slug, password)
			if err != nil {
				log.Errorf("[PublicShare] ValidateSharePassword failed: %v", err)
				http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
				return
			}
			if !valid {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"hasPassword": true,
					"error":       "Password required or invalid",
				})
				return
			}
		}

		// Retrieve minified library item details
		minItem, err := idb.GetLibraryItemMinifiedByID(db, s.LibraryItemID)
		if err != nil {
			log.Errorf("[PublicShare] GetLibraryItemMinifiedByID failed for %s: %v", s.LibraryItemID, err)
			http.Error(w, `{"error": "Shared item not found"}`, http.StatusNotFound)
			return
		}

		resPayload := map[string]interface{}{
			"id":             s.ID,
			"libraryItemId":  s.LibraryItemID,
			"isDownloadable": s.IsDownloadable,
			"hasPassword":    hasPassword,
			"item":           minItem,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resPayload)
	}
}

// handleGetPublicShareCover resolves GET /api/s/{slug}/cover
func handleGetPublicShareCover(db *sql.DB, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		parts := strings.Split(r.URL.Path, "/")
		var slug string
		for i, part := range parts {
			if part == "s" && i+1 < len(parts) {
				slug = parts[i+1]
				break
			}
		}
		if slug == "" {
			http.Error(w, `{"error": "Invalid Slug"}`, http.StatusBadRequest)
			return
		}

		s, err := globalShareManager.GetShare(r.Context(), slug)
		if err != nil || s == nil {
			http.Error(w, `{"error": "Share not found or expired"}`, http.StatusNotFound)
			return
		}

		// Check password
		if s.PasswordHash != "" {
			password := r.URL.Query().Get("password")
			if password == "" {
				password = r.Header.Get("X-Share-Password")
			}
			valid, err := globalShareManager.ValidateSharePassword(r.Context(), slug, password)
			if err != nil || !valid {
				http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
				return
			}
		}

		// Serve the cover
		coverPath, err := idb.GetCoverPath(db, s.LibraryItemID)
		if err != nil || coverPath == "" {
			http.NotFound(w, r)
			return
		}

		if !utils.IsSafeFilePath(db, metadataPath, coverPath) {
			log.Warnf("[PublicShareCover] Cover path traversal blocked: %s", coverPath)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// Serve raw or thumbnail
		raw := r.URL.Query().Get("raw") == "1"
		if raw {
			if r.URL.Query().Get("ts") != "" {
				w.Header().Set("Cache-Control", "private, max-age=86400")
			}
			http.ServeFile(w, r, coverPath)
			return
		}

		format := r.URL.Query().Get("format")
		if format == "" {
			if strings.Contains(r.Header.Get("Accept"), "image/webp") {
				format = "webp"
			} else {
				format = "jpeg"
			}
		}
		width := r.URL.Query().Get("width")
		if width == "" {
			width = "400"
		}
		height := r.URL.Query().Get("height")

		// Validate parameters to prevent path traversal and ffmpeg parameter injection
		for _, char := range width {
			if char < '0' || char > '9' {
				http.Error(w, "Invalid width", http.StatusBadRequest)
				return
			}
		}
		for _, char := range height {
			if char < '0' || char > '9' {
				http.Error(w, "Invalid height", http.StatusBadRequest)
				return
			}
		}
		if format != "webp" && format != "jpeg" && format != "jpg" && format != "png" {
			http.Error(w, "Invalid format", http.StatusBadRequest)
			return
		}

		cachePath, err := getCoverFromCache(metadataPath, s.LibraryItemID, width, height, format)
		if err == nil {
			if r.URL.Query().Get("ts") != "" {
				w.Header().Set("Cache-Control", "private, max-age=86400")
			}
			w.Header().Set("Content-Type", "image/"+format)
			http.ServeFile(w, r, cachePath)
			return
		}

		// Cache miss: generate the resized cover
		cacheFilename := s.LibraryItemID + "_" + width
		if height != "" {
			cacheFilename += "x" + height
		}
		cacheFilename += "." + format
		cachePath = filepath.Join(metadataPath, "cache", "covers", cacheFilename)

		errResize := resizeImage(coverPath, cachePath, width, height, format)
		if errResize == nil {
			if r.URL.Query().Get("ts") != "" {
				w.Header().Set("Cache-Control", "private, max-age=86400")
			}
			w.Header().Set("Content-Type", "image/"+format)
			http.ServeFile(w, r, cachePath)
			return
		}
		log.Errorf("[PublicShareCover] Resize failed for item %s: %v. Falling back to raw cover.", s.LibraryItemID, errResize)

		// Fallback to serving raw cover
		w.Header().Set("Content-Type", "image/jpeg")
		http.ServeFile(w, r, coverPath)
	}
}

// handleGetPublicShareDownload resolves GET /api/s/{slug}/download
func handleGetPublicShareDownload(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		parts := strings.Split(r.URL.Path, "/")
		var slug string
		for i, part := range parts {
			if part == "s" && i+1 < len(parts) {
				slug = parts[i+1]
				break
			}
		}
		if slug == "" {
			http.Error(w, `{"error": "Invalid Slug"}`, http.StatusBadRequest)
			return
		}

		s, err := globalShareManager.GetShare(r.Context(), slug)
		if err != nil || s == nil {
			http.Error(w, `{"error": "Share not found or expired"}`, http.StatusNotFound)
			return
		}

		if !s.IsDownloadable {
			http.Error(w, `{"error": "Download is disabled for this share link"}`, http.StatusForbidden)
			return
		}

		// Check password
		if s.PasswordHash != "" {
			password := r.URL.Query().Get("password")
			if password == "" {
				password = r.Header.Get("X-Share-Password")
			}
			valid, err := globalShareManager.ValidateSharePassword(r.Context(), slug, password)
			if err != nil || !valid {
				http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
				return
			}
		}

		info, err := idb.GetLibraryItemDownloadInfo(db, s.LibraryItemID)
		if err != nil {
			http.Error(w, `{"error": "Library item not found"}`, http.StatusNotFound)
			return
		}

		if !utils.IsSafeFilePath(db, MetadataPath, info.Path) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		if info.IsFile {
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(info.RelPath)))
			http.ServeFile(w, r, info.Path)
			return
		}

		// Directory zip downloads
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q.zip", filepath.Base(info.Path)))
		if err := streamDirAsZip(w, info.Path); err != nil {
			log.Errorf("[PublicShare] Directory zip failed: %v", err)
		}
	}
}

// handleGetPublicShareStream resolves GET /api/s/{slug}/stream
func handleGetPublicShareStream(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		parts := strings.Split(r.URL.Path, "/")
		var slug string
		for i, part := range parts {
			if part == "s" && i+1 < len(parts) {
				slug = parts[i+1]
				break
			}
		}
		if slug == "" {
			http.Error(w, `{"error": "Invalid Slug"}`, http.StatusBadRequest)
			return
		}

		s, err := globalShareManager.GetShare(r.Context(), slug)
		if err != nil || s == nil {
			http.Error(w, `{"error": "Share not found or expired"}`, http.StatusNotFound)
			return
		}

		// Check password
		if s.PasswordHash != "" {
			password := r.URL.Query().Get("password")
			if password == "" {
				password = r.Header.Get("X-Share-Password")
			}
			valid, err := globalShareManager.ValidateSharePassword(r.Context(), slug, password)
			if err != nil || !valid {
				http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
				return
			}
		}

		info, err := idb.GetLibraryItemDownloadInfo(db, s.LibraryItemID)
		if err != nil {
			http.Error(w, `{"error": "Library item not found"}`, http.StatusNotFound)
			return
		}

		if !utils.IsSafeFilePath(db, MetadataPath, info.Path) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		if info.IsFile {
			http.ServeFile(w, r, info.Path)
			return
		}

		// If it's a directory, stream the specific file requested (or default to the first audio file)
		var filename string
		trackParam := r.URL.Query().Get("track")
		if trackParam != "" {
			filename = trackParam
		}

		// If filename is empty, let's scan directory for audio files and take the first one sorted alphabetically
		if filename == "" {
			files, err := os.ReadDir(info.Path)
			if err == nil {
				for _, f := range files {
					if !f.IsDir() {
						ext := strings.ToLower(filepath.Ext(f.Name()))
						if ext == ".mp3" || ext == ".m4b" || ext == ".m4a" || ext == ".aac" || ext == ".flac" {
							filename = f.Name()
							break
						}
					}
				}
			}
		}

		if filename == "" {
			http.Error(w, `{"error": "No streamable audio files found"}`, http.StatusNotFound)
			return
		}

		// Clean path to prevent directory traversal
		targetPath := filepath.Clean(filepath.Join(info.Path, filename))
		if !utils.IsSameOrSubPath(info.Path, targetPath) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		http.ServeFile(w, r, targetPath)
	}
}
