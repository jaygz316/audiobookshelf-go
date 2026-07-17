package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	log "audiobookshelf/internal/logger"
)

// handleGetAllLibraryItemsInProgress handles GET /api/me/items-in-progress
func handleGetAllLibraryItemsInProgress(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/me/items-in-progress")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// Limit query param
		limit := 25
		if qLimit := r.URL.Query().Get("limit"); qLimit != "" {
			var l int
			if _, err := fmt.Sscanf(qLimit, "%d", &l); err == nil && l > 0 {
				limit = l
			}
		}

		rows, err := db.QueryContext(r.Context(), `SELECT id, mediaItemId, mediaItemType, duration, currentTime, isFinished, hideFromContinueListening, ebookLocation, ebookProgress, finishedAt, extraData, createdAt, updatedAt, podcastId 
			FROM mediaProgresses WHERE userId = ? AND isFinished = 0 AND (currentTime > 0 OR ebookProgress > 0) AND (hideFromContinueListening = 0)
			ORDER BY updatedAt DESC LIMIT ?`, userSess.ID, limit)
		if err != nil {
			log.Errorf("[Me Progress] In progress query error: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		libraryItemIDs := []string{}
		type localProgress struct {
			libraryItemID string
			updatedAtMs   int64
		}
		progressList := []localProgress{}

		for rows.Next() {
			var id, mediaItemId, mediaItemType string
			var duration, currentTime float64
			var isFinishedInt, hideFromContinueListeningInt int
			var ebookLocation, finishedAt, extraData, createdAt, updatedAt, podcastId sql.NullString
			var ebookProgress sql.NullFloat64

			err := rows.Scan(&id, &mediaItemId, &mediaItemType, &duration, &currentTime, &isFinishedInt, &hideFromContinueListeningInt, &ebookLocation, &ebookProgress, &finishedAt, &extraData, &createdAt, &updatedAt, &podcastId)
			if err != nil {
				log.Warnf("[Me Progress] Failed to scan progress row: %v", err)
				continue
			}

			var extra map[string]interface{}
			if extraData.Valid && extraData.String != "" {
				json.Unmarshal([]byte(extraData.String), &extra)
			}
			if extra == nil {
				continue
			}

			libItemID, _ := extra["libraryItemId"].(string)
			if libItemID != "" {
				libraryItemIDs = append(libraryItemIDs, libItemID)
				progressList = append(progressList, localProgress{
					libraryItemID: libItemID,
					updatedAtMs:   idb.ParseTimeStr(updatedAt.String),
				})
			}
		}
		if err := rows.Err(); err != nil {
			log.Errorf("[Me Progress] Progresses iteration error: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		var items []interface{} = []interface{}{}
		if len(libraryItemIDs) > 0 {
			// Query the items minified
			qPlaceholders := make([]string, len(libraryItemIDs))
			qArgs := make([]interface{}, len(libraryItemIDs))
			for i, lid := range libraryItemIDs {
				qPlaceholders[i] = "?"
				qArgs[i] = lid
			}
			query := fmt.Sprintf("SELECT id, mediaType, mediaId, title FROM libraryItems WHERE id IN (%s)", strings.Join(qPlaceholders, ","))
			itemRows, err := db.QueryContext(r.Context(), query, qArgs...)
			if err != nil {
				log.Errorf("[Me Progress] Failed to query library items: %v", err)
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			defer itemRows.Close()

			for itemRows.Next() {
				var id, mediaType, mediaId, title string
				if err := itemRows.Scan(&id, &mediaType, &mediaId, &title); err != nil {
					log.Warnf("[Me Progress] Failed to scan library item row: %v", err)
					continue
				}
				// Match updatedAt
				var lastUp int64
				for _, p := range progressList {
					if p.libraryItemID == id {
						lastUp = p.updatedAtMs
						break
					}
				}
				// Construct minified item JSON
				items = append(items, map[string]interface{}{
					"id":                 id,
					"mediaType":          mediaType,
					"mediaId":            mediaId,
					"title":              title,
					"progressLastUpdate": lastUp,
				})
			}
			if err := itemRows.Err(); err != nil {
				log.Errorf("[Me Progress] Library items query iteration error: %v", err)
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"libraryItems": items,
		})
	}
}
