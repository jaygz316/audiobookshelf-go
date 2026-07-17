package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/utils"
)

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

		if s.MaxDownloads > 0 && s.DownloadsCount >= s.MaxDownloads {
			http.Error(w, `{"error": "Download limit reached for this share link"}`, http.StatusForbidden)
			return
		}

		if err := globalShareManager.IncrementDownloadsCount(r.Context(), s.ID); err != nil {
			log.Errorf("[PublicShare] Failed to increment downloads count for %s: %v", s.ID, err)
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
