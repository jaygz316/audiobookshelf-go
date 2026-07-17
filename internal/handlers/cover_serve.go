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

func serveCover(db *sql.DB, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		var itemID string
		for i, part := range parts {
			if part == "items" && i+1 < len(parts) {
				itemID = parts[i+1]
				break
			}
		}

		if itemID == "" || strings.Contains(itemID, "..") || strings.Contains(itemID, "\\") {
			http.Error(w, `{"error": "Invalid Item ID"}`, http.StatusBadRequest)
			return
		}

		raw := r.URL.Query().Get("raw") == "1"

		if raw {
			coverPath, err := idb.GetCoverPath(db, itemID)
			if err != nil || coverPath == "" {
				http.NotFound(w, r)
				return
			}
			if !utils.IsSafeFilePath(db, metadataPath, coverPath) {
				log.Warnf("[Cover] Raw cover path traversal blocked: %s", coverPath)
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
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

		cachePath, err := getCoverFromCache(metadataPath, itemID, width, height, format)
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

		coverPath, err := idb.GetCoverPath(db, itemID)
		if err == nil && coverPath != "" {
			if !utils.IsSafeFilePath(db, metadataPath, coverPath) {
				log.Warnf("[Cover] Resized cover source path traversal blocked: %s", coverPath)
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			cacheFilename := itemID + "_" + width
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
			log.Errorf("[Cover] Resize failed for item %s: %v. Falling back to raw cover.", itemID, errResize)
		}

		log.Infof("[Cover] Cache miss. Serving raw cover.")
		if err != nil || coverPath == "" {
			http.NotFound(w, r)
			return
		}
		if !utils.IsSafeFilePath(db, metadataPath, coverPath) {
			log.Warnf("[Cover] Cache miss fallback cover path traversal blocked: %s", coverPath)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		http.ServeFile(w, r, coverPath)
	}
}
