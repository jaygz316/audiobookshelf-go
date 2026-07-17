package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	log "audiobookshelf/internal/logger"
	isocket "audiobookshelf/internal/socket"
	"audiobookshelf/internal/utils"
)

// createUpdateMeProgressPayload represents the JSON payload for creating or updating me progress.
type createUpdateMeProgressPayload struct {
	Duration                      *float64 `json:"duration"`
	CurrentTime                   *float64 `json:"currentTime"`
	IsFinished                    *bool    `json:"isFinished"`
	Progress                      *float64 `json:"progress"`
	EbookLocation                 *string  `json:"ebookLocation"`
	EbookProgress                 *float64 `json:"ebookProgress"`
	LastUpdate                    *int64   `json:"lastUpdate"`
	HideFromContinueListening     *bool    `json:"hideFromContinueListening"`
	MarkAsFinishedPercentComplete *float64 `json:"markAsFinishedPercentComplete"`
	MarkAsFinishedTimeRemaining   *float64 `json:"markAsFinishedTimeRemaining"`
	FinishedAt                    *int64   `json:"finishedAt"`
}

// handleCreateUpdateMeProgress handles PATCH /api/me/progress/:libraryItemId/:episodeId?
func handleCreateUpdateMeProgress(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] PATCH %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		subPath := utils.TrimAPIPath(r.URL.Path, "/api/me/progress/")
		libraryItemID, _, mediaItemID, mediaItemType, podcastID, status, err := resolveMediaItemFromPath(r.Context(), db, subPath)
		if err != nil {
			if status != 0 {
				http.Error(w, err.Error(), status)
			} else {
				http.Error(w, "Database error", http.StatusInternalServerError)
			}
			return
		}

		var payload createUpdateMeProgressPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		// Check if record exists
		progressID, currDuration, currCurrentTime, currIsFinished, currHideFromContinueListening, currEbookLocation, currFinishedAt, currExtraData, _, _, currEbookProgress, exists, err := queryMediaProgress(r.Context(), db, userSess.ID, mediaItemID)
		if err != nil {
			log.Errorf("[Me Progress] Lookup error: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		now := time.Now()
		nowStr := idb.TimeToDBStr(now)

		currInfo := currentProgressInfo{
			exists:                    exists,
			duration:                  currDuration,
			currentTime:               currCurrentTime,
			isFinished:                currIsFinished,
			hideFromContinueListening: currHideFromContinueListening,
			ebookLocation:             currEbookLocation,
			finishedAt:                currFinishedAt,
			extraData:                 currExtraData,
			ebookProgress:             currEbookProgress,
		}

		updated, err := calculateUpdatedProgress(&payload, currInfo, libraryItemID, now)
		if err != nil {
			log.Errorf("[Me Progress] Calculation error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		err = saveMediaProgress(r.Context(), db, exists, progressID, userSess.ID, mediaItemID, mediaItemType, updated.duration, updated.currentTime, updated.isFinished, updated.hideFromContinueListening, updated.ebookLocationNullable, updated.ebookProgress, updated.finishedAtNullable, updated.extraBytes, podcastID, nowStr, updated.updatedAtStr)
		if err != nil {
			log.Errorf("[Me Progress] Save error: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		if updated.isFinished {
			handleAutoDeletePlayedEpisode(r.Context(), db, mediaItemID)
		}

		// Update active playback session if currentTime changed
		if payload.CurrentTime != nil {
			err = updateOrCreatePlaybackSession(r.Context(), db, userSess.ID, mediaItemID, mediaItemType, libraryItemID, updated.currentTime, nowStr)
			if err != nil {
				// Logged inside updateOrCreatePlaybackSession, carry on
			}
		}

		// Broadcast update
		user, err := idb.GetUserFullByID(r.Context(), db, userSess.ID)
		if err == nil && user != nil {
			userJSON := user.ToOldJSONForBrowser(user.Type != "root")
			if isocket.GlobalAuth != nil {
				isocket.GlobalAuth.BroadcastToUser(userSess.ID, "user_updated", userJSON)
			}
		}

		w.WriteHeader(http.StatusOK)
	}
}
