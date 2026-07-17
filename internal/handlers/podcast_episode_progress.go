package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"

	"github.com/google/uuid"
)

func handleBulkUpdateEpisodesProgress(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		var podcastID, libraryItemID string
		err := db.QueryRow(`
			SELECT p.id, li.id
			FROM podcasts p
			JOIN libraryItems li ON li.mediaId = p.id AND li.mediaType = 'podcast'
			WHERE p.id = ? OR li.id = ?
		`, id, id).Scan(&podcastID, &libraryItemID)
		if err != nil {
			http.Error(w, `{"error": "Podcast not found"}`, http.StatusNotFound)
			return
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, `{"error": "Failed to read request body"}`, http.StatusBadRequest)
			return
		}

		var episodeIDs []string
		var isFinishedVal bool = true
		var currentTimeVal float64 = 0
		var durationVal float64 = 0

		var objReq struct {
			EpisodeIDs  []string `json:"episodeIds"`
			IsFinished  *bool    `json:"isFinished"`
			CurrentTime *float64 `json:"currentTime"`
			Progress    *float64 `json:"progress"`
			Duration    *float64 `json:"duration"`
		}
		if json.Unmarshal(bodyBytes, &objReq) == nil && len(objReq.EpisodeIDs) > 0 {
			episodeIDs = objReq.EpisodeIDs
			if objReq.IsFinished != nil {
				isFinishedVal = *objReq.IsFinished
			}
			if objReq.CurrentTime != nil {
				currentTimeVal = *objReq.CurrentTime
			}
			if objReq.Duration != nil {
				durationVal = *objReq.Duration
			}
		} else {
			var arrReq []string
			if json.Unmarshal(bodyBytes, &arrReq) == nil && len(arrReq) > 0 {
				episodeIDs = arrReq
			} else {
				http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
				return
			}
		}

		nowStr := idb.TimeToDBStr(time.Now())

		for _, epID := range episodeIDs {
			var count int
			_ = db.QueryRowContext(r.Context(), "SELECT count(*) FROM podcastEpisodes WHERE id = ? AND podcastId = ?", epID, podcastID).Scan(&count)
			if count == 0 {
				continue
			}

			currDuration := durationVal
			if currDuration == 0 {
				var audioFileStr string
				_ = db.QueryRowContext(r.Context(), "SELECT audioFile FROM podcastEpisodes WHERE id = ?", epID).Scan(&audioFileStr)
				if audioFileStr != "" {
					var af map[string]interface{}
					if json.Unmarshal([]byte(audioFileStr), &af) == nil && af != nil {
						if dur, ok := af["duration"].(float64); ok {
							currDuration = dur
						}
					}
				}
			}

			var progressID string
			err = db.QueryRowContext(r.Context(), "SELECT id FROM mediaProgresses WHERE userId = ? AND mediaItemId = ?", user.ID, epID).Scan(&progressID)
			if err == sql.ErrNoRows {
				progressID = uuid.New().String()
				query := `INSERT INTO mediaProgresses (id, userId, mediaItemId, mediaItemType, duration, currentTime, isFinished, hideFromContinueListening, ebookLocation, ebookProgress, finishedAt, extraData, podcastId, createdAt, updatedAt)
					VALUES (?, ?, ?, 'podcastEpisode', ?, ?, ?, 0, NULL, NULL, ?, NULL, ?, ?, ?)`
				var finishedAtVal interface{} = nil
				if isFinishedVal {
					finishedAtVal = nowStr
				}
				_, err = db.ExecContext(r.Context(), query, progressID, user.ID, epID, currDuration, currentTimeVal, explicitInt(isFinishedVal), finishedAtVal, podcastID, nowStr, nowStr)
				if err != nil {
					log.Errorf("[BulkProgress] Insert failed: %v", err)
				}
			} else if err == nil {
				query := `UPDATE mediaProgresses SET duration = ?, currentTime = ?, isFinished = ?, finishedAt = ?, updatedAt = ? WHERE id = ?`
				var finishedAtVal interface{} = nil
				if isFinishedVal {
					finishedAtVal = nowStr
				}
				_, err = db.ExecContext(r.Context(), query, currDuration, currentTimeVal, explicitInt(isFinishedVal), finishedAtVal, nowStr, progressID)
				if err != nil {
					log.Errorf("[BulkProgress] Update failed: %v", err)
				}
			}
		}

		itemMin, err := idb.GetLibraryItemMinifiedByID(db, libraryItemID)
		if err == nil && itemMin != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(itemMin)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}
}
