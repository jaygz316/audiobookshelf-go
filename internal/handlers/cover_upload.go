package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	isocket "audiobookshelf/internal/socket"
)

func handleUploadCover(db *sql.DB, cfg *core.Config, itemID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST /api/items/%s/cover", itemID)

		if strings.Contains(itemID, "..") || strings.Contains(itemID, "/") || strings.Contains(itemID, "\\") {
			http.Error(w, `{"error": "Invalid item ID"}`, http.StatusBadRequest)
			return
		}

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

		if err := r.ParseMultipartForm(10 * 1024 * 1024); err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Multipart form parse failed: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}
		defer r.MultipartForm.RemoveAll()

		file, header, err := r.FormFile("cover")
		if err != nil {
			file, header, err = r.FormFile("file")
		}
		if err != nil {
			file, header, err = r.FormFile("image")
		}
		if err != nil {
			http.Error(w, `{"error": "No cover file uploaded"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()

		var mediaType, mediaID, itemPath string
		var isFile int
		err = db.QueryRow("SELECT mediaType, mediaId, path, isFile FROM libraryItems WHERE id = ?", itemID).Scan(&mediaType, &mediaID, &itemPath, &isFile)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		ext := filepath.Ext(header.Filename)
		if ext == "" {
			ext = ".jpg"
		}
		extLower := strings.ToLower(ext)
		if extLower != ".jpg" && extLower != ".jpeg" && extLower != ".png" && extLower != ".webp" && extLower != ".gif" {
			ext = ".jpg"
		}

		destPath, err := determineCoverDestPath(db, cfg.MetadataPath, itemID, mediaType, mediaID, itemPath, isFile, ext)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		destPath, err = saveCoverFile(db, cfg.MetadataPath, itemID, destPath, ext, file)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		err = updateCoverDatabaseAndClearCache(db, cfg.MetadataPath, itemID, mediaType, mediaID, destPath)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		if isocket.GlobalAuth != nil {
			if minItem, err := idb.GetLibraryItemMinifiedByID(db, itemID); err == nil {
				EmitLibraryItemEvent("item_updated", minItem)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":   true,
			"coverPath": destPath,
		})
	}
}
