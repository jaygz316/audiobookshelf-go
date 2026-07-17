package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/utils"
)

// handleGetAuthorImage serves the author image file
func handleGetAuthorImage(db *sql.DB, metadataPath string, authorID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/authors/%s/image", authorID)

		imagePath, err := idb.GetAuthorImagePath(db, authorID)
		if err != nil || imagePath == "" {
			http.NotFound(w, r)
			return
		}

		var fullPath string
		if filepath.IsAbs(imagePath) {
			fullPath = imagePath
		} else {
			fullPath = filepath.Join(metadataPath, imagePath)
		}

		if _, err := os.Stat(fullPath); err != nil {
			log.Warnf("[Go] Author image not found: %s", fullPath)
			http.NotFound(w, r)
			return
		}

		if !utils.IsSafeFilePath(db, metadataPath, fullPath) {
			log.Warnf("[Go] Author image path traversal blocked: %s", fullPath)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if r.URL.Query().Get("ts") != "" {
			w.Header().Set("Cache-Control", "private, max-age=86400")
		}
		http.ServeFile(w, r, fullPath)
	}
}

// handleDeleteAuthorImage handles DELETE /api/authors/{id}/image
func handleDeleteAuthorImage(db *sql.DB, cfg *core.Config, authorID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] DELETE /api/authors/%s/image", authorID)

		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)
		if user.Type != "root" && user.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		imagePath, err := idb.GetAuthorImagePath(db, authorID)
		if err != nil {
			http.Error(w, "failed to check author image: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if imagePath != "" {
			var fullPath string
			if filepath.IsAbs(imagePath) {
				fullPath = imagePath
			} else {
				fullPath = filepath.Join(cfg.MetadataPath, imagePath)
			}
			if utils.IsSafeFilePath(db, cfg.MetadataPath, fullPath) {
				_ = os.Remove(fullPath)
			} else {
				log.Warnf("[DeleteAuthorImage] Blocked deletion of unsafe author image path: %s", fullPath)
			}
		}

		nowStr := time.Now().Format("2006-01-02 15:04:05.000")
		_, err = db.Exec("UPDATE authors SET imagePath = '', updatedAt = ? WHERE id = ?", nowStr, authorID)
		if err != nil {
			http.Error(w, "failed to update database: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"updated": true}`))
	}
}
