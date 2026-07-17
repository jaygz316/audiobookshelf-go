package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	idb "audiobookshelf/internal/db"
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
			"maxDownloads":   s.MaxDownloads,
			"downloadsCount": s.DownloadsCount,
			"embeddable":     s.Embeddable,
			"item":           minItem,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resPayload)
	}
}
