package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
	"audiobookshelf/internal/utils"
)

// handleGetMeProgress retrieves a specific media progress object
func handleGetMeProgress(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// Path format: /api/me/progress/:id/:episodeId?
		subPath := utils.TrimAPIPath(r.URL.Path, "/api/me/progress/")
		parts := strings.Split(subPath, "/")
		if len(parts) == 0 || parts[0] == "" {
			http.Error(w, `{"error": "Bad Request"}`, http.StatusBadRequest)
			return
		}

		libraryItemID := parts[0]
		var episodeID string
		if len(parts) > 1 {
			episodeID = parts[1]
		}

		if episodeID == "" {
			var mediaItemID, mediaItemType string
			err := db.QueryRowContext(r.Context(), "SELECT mediaId, mediaType FROM libraryItems WHERE id = ? OR mediaId = ?", libraryItemID, libraryItemID).Scan(&mediaItemID, &mediaItemType)
			if err == nil && mediaItemType == "podcast" {
				rows, err := db.QueryContext(r.Context(), `SELECT id, userId, mediaItemId, mediaItemType, duration, currentTime, isFinished, hideFromContinueListening, ebookLocation, ebookProgress, finishedAt, extraData, createdAt, updatedAt, podcastId 
					FROM mediaProgresses WHERE userId = ? AND (podcastId = ? OR json_extract(extraData, '$.libraryItemId') = ?)`, userSess.ID, mediaItemID, libraryItemID)
				if err != nil {
					log.Errorf("[Me Progress] Podcast query error: %v", err)
					http.Error(w, "Database error", http.StatusInternalServerError)
					return
				}
				defer rows.Close()
				progresses, err := scanMediaProgressRows(rows)
				if err != nil {
					log.Errorf("[Me Progress] Podcast scan error: %v", err)
					http.Error(w, "Database error", http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(progresses)
				return
			}
		}

		var row *sql.Row
		if episodeID != "" {
			row = db.QueryRowContext(r.Context(), `SELECT id, userId, mediaItemId, mediaItemType, duration, currentTime, isFinished, hideFromContinueListening, ebookLocation, ebookProgress, finishedAt, extraData, createdAt, updatedAt, podcastId 
				FROM mediaProgresses WHERE userId = ? AND mediaItemId = ?`, userSess.ID, episodeID)
		} else {
			row = db.QueryRowContext(r.Context(), `SELECT id, userId, mediaItemId, mediaItemType, duration, currentTime, isFinished, hideFromContinueListening, ebookLocation, ebookProgress, finishedAt, extraData, createdAt, updatedAt, podcastId 
				FROM mediaProgresses WHERE userId = ? AND (mediaItemId = ? OR json_extract(extraData, '$.libraryItemId') = ?)`, userSess.ID, libraryItemID, libraryItemID)
		}

		progressMap, err := scanMediaProgress(row)
		if err != nil {
			log.Errorf("[Me Progress] Scan error: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}
		if progressMap == nil {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(progressMap)
	}
}
