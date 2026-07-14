package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	isocket "audiobookshelf/internal/socket"
	"audiobookshelf/internal/utils"
)

// handleGetMe returns the logged-in user details
func handleGetMe(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/me")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		user, err := idb.GetUserFullByID(r.Context(), db, userSess.ID)
		if err != nil || user == nil {
			log.Errorf("[Me] idb.User lookup failed: %v", err)
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user.ToOldJSONForBrowser(user.Type != "root"))
	}
}

// handleUpdateMePassword allows the user to update their password
func handleUpdateMePassword(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] PATCH /api/me/password")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		if userSess.Type == "guest" {
			http.Error(w, `{"error": "Guest users cannot change password"}`, http.StatusForbidden)
			return
		}

		var body struct {
			Password    string `json:"password"`
			NewPassword string `json:"newPassword"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		user, err := idb.GetUserFullByID(r.Context(), db, userSess.ID)
		if err != nil || user == nil {
			http.Error(w, `{"error": "idb.User not found"}`, http.StatusNotFound)
			return
		}

		err = bcrypt.CompareHashAndPassword([]byte(user.Pash), []byte(body.Password))
		if err != nil {
			log.Warnf("[Me] Invalid current password for user %s", user.Username)
			http.Error(w, `{"error": "Invalid current password"}`, http.StatusBadRequest)
			return
		}

		hashed, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), 8)
		if err != nil {
			http.Error(w, `{"error": "Failed to hash password"}`, http.StatusInternalServerError)
			return
		}

		_, err = db.ExecContext(r.Context(), "UPDATE users SET pash = ?, updatedAt = ? WHERE id = ?", string(hashed), idb.TimeToDBStr(time.Now()), user.ID)
		if err != nil {
			log.Errorf("[Me] Password update DB error: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

// trimPathPrefix extracts the part of path after prefix, ignoring any router base path.
func trimPathPrefix(path, prefix string) string {
	if idx := strings.Index(path, prefix); idx != -1 {
		return path[idx+len(prefix):]
	}
	return strings.TrimPrefix(path, prefix)
}

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

		var mediaItemID, mediaItemType string
		var podcastID sql.NullString

		if episodeID != "" {
			err := db.QueryRowContext(r.Context(), "SELECT id, podcastId FROM podcastEpisodes WHERE id = ?", episodeID).Scan(&mediaItemID, &podcastID)
			if err == sql.ErrNoRows {
				http.Error(w, "Episode not found", http.StatusNotFound)
				return
			} else if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			mediaItemType = "podcastEpisode"
		} else {
			var mediaType string
			err := db.QueryRowContext(r.Context(), "SELECT mediaId, mediaType FROM libraryItems WHERE id = ? OR mediaId = ?", libraryItemID, libraryItemID).Scan(&mediaItemID, &mediaType)
			if err == sql.ErrNoRows {
				http.Error(w, "Library item not found", http.StatusNotFound)
				return
			} else if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			if mediaType != "book" {
				http.Error(w, "Library item is not a book", http.StatusBadRequest)
				return
			}
			mediaItemType = "book"
		}

		var payload struct {
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

		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		// Check if record exists
		var progressID string
		var currDuration, currCurrentTime float64
		var currIsFinished, currHideFromContinueListening int
		var currEbookLocation, currFinishedAt, currExtraData, currCreatedAt, currUpdatedAt sql.NullString
		var currEbookProgress sql.NullFloat64

		err := db.QueryRowContext(r.Context(), `SELECT id, duration, currentTime, isFinished, hideFromContinueListening, ebookLocation, ebookProgress, finishedAt, extraData, createdAt, updatedAt 
			FROM mediaProgresses WHERE userId = ? AND mediaItemId = ?`, userSess.ID, mediaItemID).
			Scan(&progressID, &currDuration, &currCurrentTime, &currIsFinished, &currHideFromContinueListening, &currEbookLocation, &currEbookProgress, &currFinishedAt, &currExtraData, &currCreatedAt, &currUpdatedAt)

		exists := true
		if err == sql.ErrNoRows {
			exists = false
		} else if err != nil {
			log.Errorf("[Me Progress] Lookup error: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		now := time.Now()
		nowStr := idb.TimeToDBStr(now)

		// Defaults/updates
		durationVal := currDuration
		if payload.Duration != nil {
			durationVal = *payload.Duration
		}
		currentTimeVal := currCurrentTime
		if payload.CurrentTime != nil {
			currentTimeVal = *payload.CurrentTime
		}

		isFinishedVal := currIsFinished != 0
		finishedAtVal := currFinishedAt.String

		var extra map[string]interface{}
		if exists && currExtraData.Valid && currExtraData.String != "" {
			json.Unmarshal([]byte(currExtraData.String), &extra)
		}
		if extra == nil {
			extra = make(map[string]interface{})
		}
		extra["libraryItemId"] = libraryItemID

		if payload.IsFinished != nil {
			isFinishedVal = *payload.IsFinished
			if isFinishedVal && (currIsFinished == 0) {
				if payload.FinishedAt != nil {
					finishedAtVal = idb.TimeToDBStr(time.UnixMilli(*payload.FinishedAt))
				} else {
					finishedAtVal = nowStr
				}
				extra["progress"] = 1.0
			} else if !isFinishedVal && (currIsFinished != 0) {
				finishedAtVal = ""
				extra["progress"] = 0.0
				currentTimeVal = 0
			}
		} else if payload.Progress != nil {
			extra["progress"] = *payload.Progress
		} else if durationVal > 0 {
			extra["progress"] = currentTimeVal / durationVal
		}

		hideFromContinueListeningVal := currHideFromContinueListening != 0
		if payload.HideFromContinueListening != nil {
			hideFromContinueListeningVal = *payload.HideFromContinueListening
		} else if payload.CurrentTime != nil && currentTimeVal != currCurrentTime {
			// Reset hide if current time changed and hide was not explicitly specified
			hideFromContinueListeningVal = false
		}

		ebookLocationVal := currEbookLocation.String
		if payload.EbookLocation != nil {
			ebookLocationVal = *payload.EbookLocation
		}
		ebookProgressVal := currEbookProgress.Float64
		if payload.EbookProgress != nil {
			ebookProgressVal = *payload.EbookProgress
		}

		// Calculate progress pct and auto finish
		progPct := 0.0
		if durationVal > 0 {
			progPct = currentTimeVal / durationVal
		}

		shouldMarkAsFinished := false
		if durationVal > 0 {
			if payload.MarkAsFinishedPercentComplete != nil && *payload.MarkAsFinishedPercentComplete > 0 {
				shouldMarkAsFinished = (progPct * 100) > *payload.MarkAsFinishedPercentComplete
			} else {
				timeRemaining := durationVal - currentTimeVal
				timeRemLimit := 10.0
				if payload.MarkAsFinishedTimeRemaining != nil {
					timeRemLimit = *payload.MarkAsFinishedTimeRemaining
				}
				shouldMarkAsFinished = timeRemaining < timeRemLimit
			}
		}

		if !isFinishedVal && shouldMarkAsFinished {
			isFinishedVal = true
			if finishedAtVal == "" {
				finishedAtVal = nowStr
			}
			extra["progress"] = 1.0
		} else if isFinishedVal && (payload.CurrentTime != nil && currentTimeVal != currCurrentTime) && !shouldMarkAsFinished {
			isFinishedVal = false
			finishedAtVal = ""
		}

		extraBytes, _ := json.Marshal(extra)
		updatedAtStr := nowStr
		if payload.LastUpdate != nil {
			updatedAtStr = idb.TimeToDBStr(time.UnixMilli(*payload.LastUpdate))
		}

		var finishedAtNullable interface{} = nil
		if finishedAtVal != "" {
			finishedAtNullable = finishedAtVal
		}

		var ebookLocationNullable interface{} = nil
		if ebookLocationVal != "" {
			ebookLocationNullable = ebookLocationVal
		}

		if exists {
			_, err = db.ExecContext(r.Context(), `UPDATE mediaProgresses SET duration = ?, currentTime = ?, isFinished = ?, hideFromContinueListening = ?, ebookLocation = ?, ebookProgress = ?, finishedAt = ?, extraData = ?, updatedAt = ? WHERE id = ?`,
				durationVal, currentTimeVal, func() int {
					if isFinishedVal {
						return 1
					}
					return 0
				}(), func() int {
					if hideFromContinueListeningVal {
						return 1
					}
					return 0
				}(), ebookLocationNullable, ebookProgressVal, finishedAtNullable, string(extraBytes), updatedAtStr, progressID)
		} else {
			progressID = uuid.New().String()
			_, err = db.ExecContext(r.Context(), `INSERT INTO mediaProgresses (id, userId, mediaItemId, mediaItemType, duration, currentTime, isFinished, hideFromContinueListening, ebookLocation, ebookProgress, finishedAt, extraData, podcastId, createdAt, updatedAt) 
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				progressID, userSess.ID, mediaItemID, mediaItemType, durationVal, currentTimeVal, func() int {
					if isFinishedVal {
						return 1
					}
					return 0
				}(), func() int {
					if hideFromContinueListeningVal {
						return 1
					}
					return 0
				}(), ebookLocationNullable, ebookProgressVal, finishedAtNullable, string(extraBytes), podcastID, nowStr, updatedAtStr)
		}

		if err != nil {
			log.Errorf("[Me Progress] Save error: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		// Update active playback session if currentTime changed
		if payload.CurrentTime != nil {
			var sessID string
			var sessExtraStr sql.NullString
			errSess := db.QueryRowContext(r.Context(), `SELECT id, extraData FROM playbackSessions WHERE userId = ? AND mediaItemId = ? ORDER BY COALESCE(updatedAt, createdAt) DESC LIMIT 1`, userSess.ID, mediaItemID).Scan(&sessID, &sessExtraStr)
			if errSess == nil {
				var sessExtra map[string]interface{}
				if sessExtraStr.Valid && sessExtraStr.String != "" {
					json.Unmarshal([]byte(sessExtraStr.String), &sessExtra)
				}
				if sessExtra == nil {
					sessExtra = make(map[string]interface{})
				}
				sessExtra["libraryItemId"] = libraryItemID

				// Calculate timeListened delta
				if val, ok := sessExtra["lastTime"]; ok {
					if lf, ok2 := val.(float64); ok2 {
						delta := currentTimeVal - lf
						if delta > 0 && delta < 15 {
							currListened := 0.0
							if clVal, ok3 := sessExtra["timeListened"]; ok3 {
								if clf, ok4 := clVal.(float64); ok4 {
									currListened = clf
								}
							}
							sessExtra["timeListened"] = currListened + delta
						}
					}
				} else {
					sessExtra["timeListened"] = 0.0
				}
				sessExtra["lastTime"] = currentTimeVal

				// Add basic fallback playMethod / deviceInfo if not present
				if _, ok := sessExtra["playMethod"]; !ok {
					sessExtra["playMethod"] = "HLS"
				}
				if _, ok := sessExtra["deviceInfo"]; !ok {
					sessExtra["deviceInfo"] = "Web Client"
				}

				sessExtraBytes, _ := json.Marshal(sessExtra)
				_, errSessUpdate := db.ExecContext(r.Context(), `UPDATE playbackSessions SET extraData = ?, updatedAt = ? WHERE id = ?`, string(sessExtraBytes), nowStr, sessID)
				if errSessUpdate != nil {
					log.Errorf("[Me Progress] Failed to update playback session: %v", errSessUpdate)
				} else if isocket.GlobalAuth != nil {
					isocket.GlobalAuth.BroadcastPlaybackSessionUpdated(userSess.ID, sessID)
				}
			} else if errSess == sql.ErrNoRows {
				// Fallback: Create playback session if none exists
				var resolvedLibraryID sql.NullString
				if mediaItemType == "podcastEpisode" {
					var podcastID string
					_ = db.QueryRowContext(r.Context(), "SELECT podcastId FROM podcastEpisodes WHERE id = ?", mediaItemID).Scan(&podcastID)
					if podcastID != "" {
						_ = db.QueryRowContext(r.Context(), "SELECT libraryId FROM libraryItems WHERE mediaId = ? AND mediaType = 'podcast'", podcastID).Scan(&resolvedLibraryID)
					}
				} else {
					_ = db.QueryRowContext(r.Context(), "SELECT libraryId FROM libraryItems WHERE mediaId = ? AND mediaType = 'book'", mediaItemID).Scan(&resolvedLibraryID)
				}
				sessID := uuid.New().String()
				sessExtra := map[string]interface{}{
					"libraryItemId": libraryItemID,
					"timeListened":  0.0,
					"lastTime":      currentTimeVal,
					"playMethod":    "HLS",
					"deviceInfo":    "Web Client",
				}
				sessExtraBytes, _ := json.Marshal(sessExtra)
				_, errSessInsert := db.ExecContext(r.Context(), `INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, libraryId, extraData, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					sessID, userSess.ID, mediaItemID, mediaItemType, currentTimeVal, resolvedLibraryID, string(sessExtraBytes), nowStr, nowStr)
				if errSessInsert != nil {
					log.Errorf("[Me Progress] Failed to create fallback playback session: %v", errSessInsert)
				} else if isocket.GlobalAuth != nil {
					isocket.GlobalAuth.BroadcastPlaybackSessionAdded(userSess.ID, sessID)
				}
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

// handleRemoveMeProgress handles DELETE /api/me/progress/:id
func handleRemoveMeProgress(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] DELETE %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		progressID := utils.TrimAPIPath(r.URL.Path, "/api/me/progress/")
		if progressID == "" || strings.Contains(progressID, "/") {
			http.Error(w, `{"error": "Bad Request"}`, http.StatusBadRequest)
			return
		}

		// Verify belongs to user
		var count int
		db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM mediaProgresses WHERE id = ? AND userId = ?", progressID, userSess.ID).Scan(&count)
		if count == 0 {
			http.Error(w, `{"error": "Progress not found"}`, http.StatusNotFound)
			return
		}

		_, err := db.ExecContext(r.Context(), "DELETE FROM mediaProgresses WHERE id = ?", progressID)
		if err != nil {
			log.Errorf("[Me Progress] Delete error: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
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

// handleRemoveSeriesFromContinueListening handles GET /api/me/series/:id/remove-from-continue-listening
func handleRemoveSeriesFromContinueListening(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		sub := utils.TrimAPIPath(r.URL.Path, "/api/me/series/")
		seriesID := strings.TrimSuffix(sub, "/remove-from-continue-listening")
		if seriesID == "" || strings.Contains(seriesID, "/") {
			http.Error(w, `{"error": "Bad Request"}`, http.StatusBadRequest)
			return
		}

		// Check series exists
		var count int
		db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM series WHERE id = ?", seriesID).Scan(&count)
		if count == 0 {
			http.Error(w, `{"error": "Series not found"}`, http.StatusNotFound)
			return
		}

		user, err := idb.GetUserFullByID(r.Context(), db, userSess.ID)
		if err != nil || user == nil {
			http.Error(w, "idb.User not found", http.StatusNotFound)
			return
		}

		var extra map[string]interface{}
		if len(user.ExtraData) > 0 {
			json.Unmarshal(user.ExtraData, &extra)
		}
		if extra == nil {
			extra = make(map[string]interface{})
		}

		seriesArr, _ := extra["seriesHideFromContinueListening"].([]interface{})
		exists := false
		for _, s := range seriesArr {
			if sStr, ok := s.(string); ok && sStr == seriesID {
				exists = true
				break
			}
		}
		if !exists {
			seriesArr = append(seriesArr, seriesID)
			extra["seriesHideFromContinueListening"] = seriesArr
			extraBytes, _ := json.Marshal(extra)
			_, err = db.ExecContext(r.Context(), "UPDATE users SET extraData = ?, updatedAt = ? WHERE id = ?", string(extraBytes), idb.TimeToDBStr(time.Now()), user.ID)
			if err != nil {
				log.Errorf("[Me Series] DB error: %v", err)
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			user, _ = idb.GetUserFullByID(r.Context(), db, userSess.ID)
		}

		userJSON := user.ToOldJSONForBrowser(user.Type != "root")
		if isocket.GlobalAuth != nil {
			isocket.GlobalAuth.BroadcastToUser(userSess.ID, "user_updated", userJSON)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(userJSON)
	}
}

// handleReaddSeriesFromContinueListening handles GET /api/me/series/:id/readd-to-continue-listening
func handleReaddSeriesFromContinueListening(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		sub := utils.TrimAPIPath(r.URL.Path, "/api/me/series/")
		seriesID := strings.TrimSuffix(sub, "/readd-to-continue-listening")
		if seriesID == "" || strings.Contains(seriesID, "/") {
			http.Error(w, `{"error": "Bad Request"}`, http.StatusBadRequest)
			return
		}

		user, err := idb.GetUserFullByID(r.Context(), db, userSess.ID)
		if err != nil || user == nil {
			http.Error(w, "idb.User not found", http.StatusNotFound)
			return
		}

		var extra map[string]interface{}
		if len(user.ExtraData) > 0 {
			json.Unmarshal(user.ExtraData, &extra)
		}
		if extra == nil {
			extra = make(map[string]interface{})
		}

		seriesArr, _ := extra["seriesHideFromContinueListening"].([]interface{})
		newSeriesArr := []interface{}{}
		changed := false
		for _, s := range seriesArr {
			if sStr, ok := s.(string); ok && sStr == seriesID {
				changed = true
			} else {
				newSeriesArr = append(newSeriesArr, s)
			}
		}

		if changed {
			extra["seriesHideFromContinueListening"] = newSeriesArr
			extraBytes, _ := json.Marshal(extra)
			_, err = db.ExecContext(r.Context(), "UPDATE users SET extraData = ?, updatedAt = ? WHERE id = ?", string(extraBytes), idb.TimeToDBStr(time.Now()), user.ID)
			if err != nil {
				log.Errorf("[Me Series] DB error: %v", err)
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			user, _ = idb.GetUserFullByID(r.Context(), db, userSess.ID)
		}

		userJSON := user.ToOldJSONForBrowser(user.Type != "root")
		if isocket.GlobalAuth != nil {
			isocket.GlobalAuth.BroadcastToUser(userSess.ID, "user_updated", userJSON)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(userJSON)
	}
}

// handleHideMeProgressFromContinueListening handles GET /api/me/progress/:id/remove-from-continue-listening
func handleHideMeProgressFromContinueListening(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		sub := utils.TrimAPIPath(r.URL.Path, "/api/me/progress/")
		progressID := strings.TrimSuffix(sub, "/remove-from-continue-listening")
		progressID = strings.TrimSuffix(progressID, "/hide-from-continue-listening")
		if progressID == "" || strings.Contains(progressID, "/") {
			http.Error(w, `{"error": "Bad Request"}`, http.StatusBadRequest)
			return
		}

		_, err := db.ExecContext(r.Context(), "UPDATE mediaProgresses SET hideFromContinueListening = 1, updatedAt = ? WHERE id = ? AND userId = ?",
			idb.TimeToDBStr(time.Now()), progressID, userSess.ID)
		if err != nil {
			log.Errorf("[Me Progress] Hide progress error: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		user, err := idb.GetUserFullByID(r.Context(), db, userSess.ID)
		if err == nil && user != nil {
			userJSON := user.ToOldJSONForBrowser(user.Type != "root")
			if isocket.GlobalAuth != nil {
				isocket.GlobalAuth.BroadcastToUser(userSess.ID, "user_updated", userJSON)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(userJSON)
			return
		}

		http.Error(w, "idb.User not found", http.StatusNotFound)
	}
}

// Bookmarks routes:
// POST /api/me/item/:id/bookmark
// PATCH /api/me/item/:id/bookmark
// DELETE /api/me/item/:id/bookmark/:time

type Bookmark struct {
	LibraryItemID string  `json:"libraryItemId"`
	Time          float64 `json:"time"`
	Title         string  `json:"title"`
	CreatedAt     int64   `json:"createdAt"`
}

func handleMeCreateBookmark(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		sub := utils.TrimAPIPath(r.URL.Path, "/api/me/item/")
		libraryItemID := strings.TrimSuffix(sub, "/bookmark")
		if libraryItemID == "" || strings.Contains(libraryItemID, "/") {
			http.Error(w, `{"error": "Bad Request"}`, http.StatusBadRequest)
			return
		}

		// Read body
		var body struct {
			Time  float64 `json:"time"`
			Title string  `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		if body.Title == "" {
			http.Error(w, `{"error": "Title required"}`, http.StatusBadRequest)
			return
		}

		user, err := idb.GetUserFullByID(r.Context(), db, userSess.ID)
		if err != nil || user == nil {
			http.Error(w, "idb.User not found", http.StatusNotFound)
			return
		}

		var bookmarks []Bookmark
		if len(user.Bookmarks) > 0 {
			json.Unmarshal(user.Bookmarks, &bookmarks)
		}

		// Create new bookmark
		newBookmark := Bookmark{
			LibraryItemID: libraryItemID,
			Time:          body.Time,
			Title:         body.Title,
			CreatedAt:     time.Now().UnixMilli(),
		}
		bookmarks = append(bookmarks, newBookmark)

		bookmarksBytes, _ := json.Marshal(bookmarks)
		_, err = db.ExecContext(r.Context(), "UPDATE users SET bookmarks = ?, updatedAt = ? WHERE id = ?", string(bookmarksBytes), idb.TimeToDBStr(time.Now()), user.ID)
		if err != nil {
			log.Errorf("[Me Bookmark] DB error: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		user, _ = idb.GetUserFullByID(r.Context(), db, userSess.ID)
		userJSON := user.ToOldJSONForBrowser(user.Type != "root")
		if isocket.GlobalAuth != nil {
			isocket.GlobalAuth.BroadcastToUser(userSess.ID, "user_updated", userJSON)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(newBookmark)
	}
}

func handleMeUpdateBookmark(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] PATCH %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		sub := utils.TrimAPIPath(r.URL.Path, "/api/me/item/")
		libraryItemID := strings.TrimSuffix(sub, "/bookmark")
		if libraryItemID == "" || strings.Contains(libraryItemID, "/") {
			http.Error(w, `{"error": "Bad Request"}`, http.StatusBadRequest)
			return
		}

		// Read body
		var body struct {
			Time  float64 `json:"time"`
			Title string  `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		user, err := idb.GetUserFullByID(r.Context(), db, userSess.ID)
		if err != nil || user == nil {
			http.Error(w, "idb.User not found", http.StatusNotFound)
			return
		}

		var bookmarks []Bookmark
		if len(user.Bookmarks) > 0 {
			json.Unmarshal(user.Bookmarks, &bookmarks)
		}

		found := false
		var updated Bookmark
		for i, b := range bookmarks {
			if b.LibraryItemID == libraryItemID && b.Time == body.Time {
				bookmarks[i].Title = body.Title
				updated = bookmarks[i]
				found = true
				break
			}
		}

		if !found {
			http.Error(w, "Bookmark not found", http.StatusNotFound)
			return
		}

		bookmarksBytes, _ := json.Marshal(bookmarks)
		_, err = db.ExecContext(r.Context(), "UPDATE users SET bookmarks = ?, updatedAt = ? WHERE id = ?", string(bookmarksBytes), idb.TimeToDBStr(time.Now()), user.ID)
		if err != nil {
			log.Errorf("[Me Bookmark] DB error: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		user, _ = idb.GetUserFullByID(r.Context(), db, userSess.ID)
		userJSON := user.ToOldJSONForBrowser(user.Type != "root")
		if isocket.GlobalAuth != nil {
			isocket.GlobalAuth.BroadcastToUser(userSess.ID, "user_updated", userJSON)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(updated)
	}
}

func handleMeRemoveBookmark(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] DELETE %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// Route format: /api/me/item/:id/bookmark/:time
		sub := utils.TrimAPIPath(r.URL.Path, "/api/me/item/")
		parts := strings.Split(sub, "/bookmark/")
		if len(parts) != 2 {
			http.Error(w, `{"error": "Bad Request"}`, http.StatusBadRequest)
			return
		}

		libraryItemID := parts[0]
		timeStr, err := url.PathUnescape(parts[1])
		if err != nil {
			http.Error(w, `{"error": "Bad Request"}`, http.StatusBadRequest)
			return
		}

		var timeVal float64
		if _, err := fmt.Sscanf(timeStr, "%f", &timeVal); err != nil {
			http.Error(w, `{"error": "Invalid time value"}`, http.StatusBadRequest)
			return
		}

		user, err := idb.GetUserFullByID(r.Context(), db, userSess.ID)
		if err != nil || user == nil {
			http.Error(w, "idb.User not found", http.StatusNotFound)
			return
		}

		var bookmarks []Bookmark
		if len(user.Bookmarks) > 0 {
			json.Unmarshal(user.Bookmarks, &bookmarks)
		}

		newBookmarks := []Bookmark{}
		found := false
		for _, b := range bookmarks {
			if b.LibraryItemID == libraryItemID && b.Time == timeVal {
				found = true
			} else {
				newBookmarks = append(newBookmarks, b)
			}
		}

		if !found {
			http.Error(w, "Bookmark not found", http.StatusNotFound)
			return
		}

		bookmarksBytes, _ := json.Marshal(newBookmarks)
		_, err = db.ExecContext(r.Context(), "UPDATE users SET bookmarks = ?, updatedAt = ? WHERE id = ?", string(bookmarksBytes), idb.TimeToDBStr(time.Now()), user.ID)
		if err != nil {
			log.Errorf("[Me Bookmark] DB error: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		user, _ = idb.GetUserFullByID(r.Context(), db, userSess.ID)
		userJSON := user.ToOldJSONForBrowser(user.Type != "root")
		if isocket.GlobalAuth != nil {
			isocket.GlobalAuth.BroadcastToUser(userSess.ID, "user_updated", userJSON)
		}

		w.WriteHeader(http.StatusOK)
	}
}

// scanMediaProgress scans a database row into a map format used by client API
func scanMediaProgress(row *sql.Row) (map[string]interface{}, error) {
	var id, userId, mediaItemId, mediaItemType string
	var duration, currentTime float64
	var isFinishedInt, hideFromContinueListeningInt int
	var ebookLocation, finishedAt, extraData, createdAt, updatedAt, podcastId sql.NullString
	var ebookProgress sql.NullFloat64

	err := row.Scan(&id, &userId, &mediaItemId, &mediaItemType, &duration, &currentTime, &isFinishedInt, &hideFromContinueListeningInt, &ebookLocation, &ebookProgress, &finishedAt, &extraData, &createdAt, &updatedAt, &podcastId)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var extra map[string]interface{}
	if extraData.Valid && extraData.String != "" {
		json.Unmarshal([]byte(extraData.String), &extra)
	}
	if extra == nil {
		extra = make(map[string]interface{})
	}

	libItemID, _ := extra["libraryItemId"].(string)
	progressVal := 0.0
	if duration > 0 {
		progressVal = currentTime / duration
		if progressVal > 1.0 {
			progressVal = 1.0
		}
	}

	updatedAtMs := idb.ParseTimeStr(updatedAt.String)
	createdAtMs := idb.ParseTimeStr(createdAt.String)
	var finishedAtMs *int64
	if finishedAt.Valid && finishedAt.String != "" {
		val := idb.ParseTimeStr(finishedAt.String)
		finishedAtMs = &val
	}

	var episodeID *string
	if mediaItemType == "podcastEpisode" {
		val := mediaItemId
		episodeID = &val
	}

	var ebookLoc *string
	if ebookLocation.Valid {
		val := ebookLocation.String
		ebookLoc = &val
	}
	var ebookProgVal *float64
	if ebookProgress.Valid {
		val := ebookProgress.Float64
		ebookProgVal = &val
	}

	return map[string]interface{}{
		"id":                        id,
		"userId":                    userId,
		"libraryItemId":             libItemID,
		"episodeId":                 episodeID,
		"mediaItemId":               mediaItemId,
		"mediaItemType":             mediaItemType,
		"duration":                  duration,
		"progress":                  progressVal,
		"currentTime":               currentTime,
		"isFinished":                isFinishedInt != 0,
		"hideFromContinueListening": hideFromContinueListeningInt != 0,
		"ebookLocation":             ebookLoc,
		"ebookProgress":             ebookProgVal,
		"lastUpdate":                updatedAtMs,
		"startedAt":                 createdAtMs,
		"finishedAt":                finishedAtMs,
	}, nil
}
