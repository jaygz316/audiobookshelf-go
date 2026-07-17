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

	"audiobookshelf/internal/core"
	"audiobookshelf/internal/utils"
)

type AudioFileInfo struct {
	Path     string
	Duration float64
}

func handleGetWaveform(db *sql.DB, cfg *core.Config, itemID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "Unauthorized"}`))
			return
		}

		// Validate itemID to prevent path traversal
		if strings.Contains(itemID, "..") || strings.Contains(itemID, "/") || strings.Contains(itemID, "\\") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": "Invalid item ID"}`))
			return
		}

		itemDir := filepath.Join(cfg.MetadataPath, "items", itemID)
		waveformPath := filepath.Join(itemDir, "waveform.json")

		if data, err := os.ReadFile(waveformPath); err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.Write(data)
			return
		}

		infos, err := getAudioFilesInfo(db, itemID)
		if err != nil {
			log.Errorf("[Waveform] Failed to resolve audio files for %s: %v", itemID, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(fmt.Sprintf(`{"error": "%v"}`, err)))
			return
		}

		// Verify safety of all audio file paths
		for _, info := range infos {
			if !utils.IsSafeFilePath(db, cfg.MetadataPath, info.Path) {
				log.Warnf("[Waveform] Unsafe audio file path traversal blocked: %s", info.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error": "Forbidden"}`))
				return
			}
		}

		// Target 200 points for the player waveform
		peaks, err := GenerateWaveform(infos, 200)
		if err != nil {
			log.Errorf("[Waveform] Failed to generate waveform for %s: %v", itemID, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf(`{"error": "Failed to generate waveform: %v"}`, err)))
			return
		}

		respData := map[string]interface{}{
			"itemId": itemID,
			"peaks":  peaks,
		}

		jsonData, err := json.Marshal(respData)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "Failed to marshal response"}`))
			return
		}

		_ = os.MkdirAll(itemDir, 0755)
		_ = os.WriteFile(waveformPath, jsonData, 0644)

		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonData)
	}
}
