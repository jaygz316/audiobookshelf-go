package handlers

import (
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/utils"
)

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
