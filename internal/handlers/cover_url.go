package handlers

import (
	log "audiobookshelf/internal/logger"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	isocket "audiobookshelf/internal/socket"
)

func handleUpdateCoverFromURL(db *sql.DB, cfg *core.Config, itemID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST /api/items/%s/cover-from-url", itemID)

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

		var body struct {
			CoverURL string `json:"coverUrl"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		if body.CoverURL == "" {
			http.Error(w, `{"error": "coverUrl is required"}`, http.StatusBadRequest)
			return
		}

		destPath, err := downloadCoverFromURL(r.Context(), db, itemID, body.CoverURL, cfg.MetadataPath)
		if err != nil {
			log.Errorf("[Cover From URL] Failed: %v", err)
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

func downloadCoverFromURL(ctx context.Context, db *sql.DB, itemID string, coverURL string, metadataPath string) (string, error) {
	if coverURL == "" {
		return "", fmt.Errorf("empty cover URL")
	}

	var mediaType, mediaID, itemPath string
	var isFile int
	err := db.QueryRow("SELECT mediaType, mediaId, path, isFile FROM libraryItems WHERE id = ?", itemID).Scan(&mediaType, &mediaID, &itemPath, &isFile)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", coverURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := coverHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch cover from URL, status: %d", resp.StatusCode)
	}

	ext := ".jpg"
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "image/png") {
		ext = ".png"
	} else if strings.Contains(contentType, "image/webp") {
		ext = ".webp"
	} else if strings.Contains(contentType, "image/gif") {
		ext = ".gif"
	}

	destPath, err := determineCoverDestPath(db, metadataPath, itemID, mediaType, mediaID, itemPath, isFile, ext)
	if err != nil {
		return "", err
	}

	destPath, err = saveCoverFile(db, metadataPath, itemID, destPath, ext, resp.Body)
	if err != nil {
		return "", err
	}

	err = updateCoverDatabaseAndClearCache(db, metadataPath, itemID, mediaType, mediaID, destPath)
	if err != nil {
		return "", err
	}

	return destPath, nil
}
