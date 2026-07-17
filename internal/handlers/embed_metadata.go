package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"audiobookshelf/internal/core"
	log "audiobookshelf/internal/logger"
	"audiobookshelf/internal/utils"
)

func handleEmbedMetadata(db *sql.DB, cfg *core.Config, itemID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST /api/items/%s/embed-metadata", itemID)

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

		meta, err := getBookMetadataForEmbedding(db, itemID)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, `{"error": "Item not found"}`, http.StatusNotFound)
				return
			}
			if err.Error() == "Only books support metadata tag embedding" || err.Error() == "No audio files found for this library item" {
				http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusBadRequest)
				return
			}
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		metaFilePath, err := writeFFMetadataFile(meta)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), http.StatusInternalServerError)
			return
		}
		defer os.Remove(metaFilePath)

		// Validate cover art path if present
		hasCover := false
		resolvedCoverPath := ""
		if meta.CoverPath != "" && utils.IsSafeFilePath(db, cfg.MetadataPath, meta.CoverPath) {
			if _, err := os.Stat(meta.CoverPath); err == nil {
				hasCover = true
				resolvedCoverPath = meta.CoverPath
			}
		}

		updatedFiles, err := embedMetadataInAudioFiles(db, cfg, meta.AudioFiles, metaFilePath, resolvedCoverPath, hasCover)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":      true,
			"message":      "Metadata, chapters, and cover art embedded successfully",
			"updatedFiles": updatedFiles,
		})
	}
}
