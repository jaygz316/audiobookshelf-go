package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"net/http"
	"path/filepath"
	"strings"

	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/utils"
)

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

		if strings.Contains(s.LibraryItemID, "..") || strings.Contains(s.LibraryItemID, "/") || strings.Contains(s.LibraryItemID, "\\") {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		cachePath, err := getCoverFromCache(metadataPath, s.LibraryItemID, width, height, format)
		if err == nil {
			if !utils.IsSafeFilePath(db, metadataPath, cachePath) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
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
