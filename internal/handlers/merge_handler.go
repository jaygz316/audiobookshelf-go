package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
)

func handleMergeAudioFiles(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
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

		path := r.URL.Path
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) < 3 || parts[len(parts)-1] != "merge" {
			http.Error(w, `{"error": "Invalid request path"}`, http.StatusBadRequest)
			return
		}
		itemID := parts[len(parts)-2]

		log.Infof("[Go] POST /api/items/%s/merge", itemID)

		mergeCtx, status, err := prepareMergeContext(db, itemID)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), status)
			return
		}

		chapters, totalDuration, status, err := runMergeFFmpeg(r.Context(), mergeCtx)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), status)
			return
		}

		status, err = updateDatabaseAndCleanup(db, mergeCtx, chapters, totalDuration)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), status)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Audio files merged successfully into a single M4B file.",
		})
	}
}
