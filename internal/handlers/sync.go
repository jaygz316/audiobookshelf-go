package handlers

import (
	log "audiobookshelf/internal/logger"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	isocket "audiobookshelf/internal/socket"
)

// LocalMediaProgressItem represents one item in the sync list
type LocalMediaProgressItem struct {
	ID                        interface{} `json:"id"`
	LibraryItemID             string      `json:"libraryItemId"`
	EpisodeID                 *string     `json:"episodeId"`
	Duration                  float64     `json:"duration"`
	Progress                  *float64    `json:"progress"`
	CurrentTime               *float64    `json:"currentTime"`
	IsFinished                bool        `json:"isFinished"`
	HideFromContinueListening bool        `json:"hideFromContinueListening"`
	UpdatedAt                 interface{} `json:"updatedAt"` // can be float64 or string
}

// LocalMediaProgressPayload is the payload of POST /api/me/sync-local-progress
type LocalMediaProgressPayload struct {
	LocalMediaProgress []LocalMediaProgressItem `json:"localMediaProgress"`
}

// LocalSessionItem represents a playback session to be synced
type LocalSessionItem struct {
	ID            string      `json:"id"`
	LibraryID     string      `json:"libraryId"`
	LibraryItemID string      `json:"libraryItemId"`
	EpisodeID     *string     `json:"episodeId"`
	TimeListening float64     `json:"timeListening"`
	StartTime     float64     `json:"startTime"`
	CurrentTime   float64     `json:"currentTime"`
	StartedAt     interface{} `json:"startedAt"`
	UpdatedAt     interface{} `json:"updatedAt"`
	Duration      float64     `json:"duration"`
	PlayMethod    interface{} `json:"playMethod"`
	MediaPlayer   string      `json:"mediaPlayer"`
	DeviceInfo    interface{} `json:"deviceInfo"`
}

// LocalSessionsPayload is the payload of POST /api/session/local-all
type LocalSessionsPayload struct {
	Sessions []LocalSessionItem `json:"sessions"`
}

// SyncSessionResult is returned for each session processed
type SyncSessionResult struct {
	ID             string `json:"id"`
	Success        bool   `json:"success"`
	ProgressSynced bool   `json:"progressSynced"`
}

// SyncSessionsResponse is the response of POST /api/session/local-all
type SyncSessionsResponse struct {
	Results []SyncSessionResult `json:"results"`
}

func parseJSONTime(val interface{}) time.Time {
	if val == nil {
		return time.Now()
	}
	switch v := val.(type) {
	case float64:
		return time.UnixMilli(int64(v))
	case int64:
		return time.UnixMilli(v)
	case string:
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
		if t, err := time.Parse("2006-01-02 15:04:05.999 Z07:00", v); err == nil {
			return t
		}
		if t, err := time.Parse("2006-01-02 15:04:05.999", v); err == nil {
			return t
		}
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
			return time.UnixMilli(ms)
		}
	}
	return time.Now()
}

func stringifyDeviceInfo(val interface{}) string {
	if val == nil {
		return "Unknown Device"
	}
	switch v := val.(type) {
	case string:
		return v
	case map[string]interface{}:
		clientName := ""
		if cn, ok := v["clientName"].(string); ok {
			clientName = cn
		} else if bn, ok := v["browserName"].(string); ok {
			clientName = bn
		}

		osName := ""
		if os, ok := v["osName"].(string); ok {
			osName = os
		}

		if clientName != "" && osName != "" {
			return clientName + " / " + osName
		} else if clientName != "" {
			return clientName
		} else if osName != "" {
			return osName
		}
	}
	return "Unknown Device"
}

func stringifyPlayMethod(val interface{}) string {
	if val == nil {
		return "HLS"
	}
	switch v := val.(type) {
	case string:
		return v
	case float64:
		switch int(v) {
		case 0:
			return "Direct Play"
		case 1:
			return "Direct Stream"
		case 2:
			return "Transcode"
		}
		return fmt.Sprintf("PlayMethod %d", int(v))
	}
	return "HLS"
}

// handleSyncLocalProgress handles POST /api/me/sync-local-progress
func handleSyncLocalProgress(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var payload LocalMediaProgressPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			log.Warnf("[Sync Progress] Decode failed: %v", err)
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		now := time.Now()
		nowStr := idb.TimeToDBStr(now)
		progressUpdatedAny := false

		for _, item := range payload.LocalMediaProgress {
			if item.LibraryItemID == "" {
				continue
			}

			var mediaItemID, mediaItemType string
			var podcastID sql.NullString

			if item.EpisodeID != nil && *item.EpisodeID != "" {
				err := db.QueryRowContext(ctx, "SELECT id, podcastId FROM podcastEpisodes WHERE id = ?", *item.EpisodeID).Scan(&mediaItemID, &podcastID)
				if err != nil {
					log.Warnf("[Sync Progress] Episode not found for episodeId %s: %v", *item.EpisodeID, err)
					continue
				}
				mediaItemType = "podcastEpisode"
			} else {
				var mediaType string
				err := db.QueryRowContext(ctx, "SELECT mediaId, mediaType FROM libraryItems WHERE id = ? OR mediaId = ?", item.LibraryItemID, item.LibraryItemID).Scan(&mediaItemID, &mediaType)
				if err != nil {
					log.Warnf("[Sync Progress] Library item not found for libraryItemId %s: %v", item.LibraryItemID, err)
					continue
				}
				if mediaType != "book" {
					log.Warnf("[Sync Progress] Library item is not a book: %s (type %s)", item.LibraryItemID, mediaType)
					continue
				}
				mediaItemType = "book"
			}

			itemUpdatedTime := parseJSONTime(item.UpdatedAt)
			itemUpdatedAtMs := itemUpdatedTime.UnixMilli()

			var progressID string
			var dbUpdatedAtStr sql.NullString

			err := db.QueryRowContext(ctx, `SELECT id, updatedAt FROM mediaProgresses WHERE userId = ? AND mediaItemId = ?`, userSess.ID, mediaItemID).Scan(&progressID, &dbUpdatedAtStr)
			exists := (err == nil)

			if exists && dbUpdatedAtStr.Valid && dbUpdatedAtStr.String != "" {
				dbUpdatedAtMs := idb.ParseTimeStr(dbUpdatedAtStr.String)
				if dbUpdatedAtMs >= itemUpdatedAtMs {
					continue
				}
			}

			currentTimeVal := 0.0
			if item.CurrentTime != nil {
				currentTimeVal = *item.CurrentTime
			} else if item.Progress != nil {
				currentTimeVal = *item.Progress
			}

			durationVal := item.Duration
			if durationVal <= 0 {
				durationVal = 0.0
			}

			progPct := 0.0
			if durationVal > 0 {
				progPct = currentTimeVal / durationVal
				if progPct > 1.0 {
					progPct = 1.0
				}
			}

			extra := map[string]interface{}{
				"libraryItemId": item.LibraryItemID,
				"progress":      progPct,
			}
			extraBytes, _ := json.Marshal(extra)

			finishedAtNullable := interface{}(nil)
			if item.IsFinished {
				finishedAtNullable = idb.TimeToDBStr(itemUpdatedTime)
			}

			updatedAtStr := idb.TimeToDBStr(itemUpdatedTime)

			if exists {
				_, err = db.ExecContext(ctx, `UPDATE mediaProgresses SET duration = ?, currentTime = ?, isFinished = ?, hideFromContinueListening = ?, finishedAt = ?, extraData = ?, updatedAt = ? WHERE id = ?`,
					durationVal, currentTimeVal, func() int {
						if item.IsFinished {
							return 1
						}
						return 0
					}(), func() int {
						if item.HideFromContinueListening {
							return 1
						}
						return 0
					}(), finishedAtNullable, string(extraBytes), updatedAtStr, progressID)
			} else {
				progressID = uuid.New().String()
				_, err = db.ExecContext(ctx, `INSERT INTO mediaProgresses (id, userId, mediaItemId, mediaItemType, duration, currentTime, isFinished, hideFromContinueListening, finishedAt, extraData, podcastId, createdAt, updatedAt) 
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					progressID, userSess.ID, mediaItemID, mediaItemType, durationVal, currentTimeVal, func() int {
						if item.IsFinished {
							return 1
						}
						return 0
					}(), func() int {
						if item.HideFromContinueListening {
							return 1
						}
						return 0
					}(), finishedAtNullable, string(extraBytes), podcastID, nowStr, updatedAtStr)
			}

			if err != nil {
				log.Errorf("[Sync Progress] Database error saving progress: %v", err)
				continue
			}

			progressUpdatedAny = true

			var sessID string
			var sessExtraStr sql.NullString
			errSess := db.QueryRowContext(ctx, `SELECT id, extraData FROM playbackSessions WHERE userId = ? AND mediaItemId = ? ORDER BY COALESCE(updatedAt, createdAt) DESC LIMIT 1`, userSess.ID, mediaItemID).Scan(&sessID, &sessExtraStr)
			if errSess == nil {
				var sessExtra map[string]interface{}
				if sessExtraStr.Valid && sessExtraStr.String != "" {
					json.Unmarshal([]byte(sessExtraStr.String), &sessExtra)
				}
				if sessExtra == nil {
					sessExtra = make(map[string]interface{})
				}
				sessExtra["libraryItemId"] = item.LibraryItemID
				sessExtra["lastTime"] = currentTimeVal
				if _, ok := sessExtra["timeListened"]; !ok {
					sessExtra["timeListened"] = 0.0
				}
				sessExtraBytes, _ := json.Marshal(sessExtra)

				_, errSessUpdate := db.ExecContext(ctx, `UPDATE playbackSessions SET extraData = ?, updatedAt = ? WHERE id = ?`, string(sessExtraBytes), updatedAtStr, sessID)
				if errSessUpdate != nil {
					log.Errorf("[Sync Progress] Failed to update playback session: %v", errSessUpdate)
				} else if isocket.GlobalAuth != nil {
					isocket.GlobalAuth.BroadcastPlaybackSessionUpdated(userSess.ID, sessID)
				}
			}
		}

		if progressUpdatedAny {
			user, err := idb.GetUserFullByID(ctx, db, userSess.ID)
			if err == nil && user != nil {
				userJSON := user.ToOldJSONForBrowser(user.Type != "root")
				if isocket.GlobalAuth != nil {
					isocket.GlobalAuth.BroadcastToUser(userSess.ID, "user_updated", userJSON)
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true}`))
	}
}

// syncSingleSession processes a single local playback session item, updating the database.
func syncSingleSession(ctx context.Context, db *sql.DB, userSess *core.UserSession, item LocalSessionItem) (SyncSessionResult, bool) {
	if item.ID == "" || item.LibraryItemID == "" {
		return SyncSessionResult{
			ID:             item.ID,
			Success:        false,
			ProgressSynced: false,
		}, false
	}

	var mediaItemID, mediaItemType string
	var podcastID sql.NullString

	if item.EpisodeID != nil && *item.EpisodeID != "" {
		err := db.QueryRowContext(ctx, "SELECT id, podcastId FROM podcastEpisodes WHERE id = ?", *item.EpisodeID).Scan(&mediaItemID, &podcastID)
		if err != nil {
			log.Warnf("[Sync Sessions] Episode not found for episodeId %s: %v", *item.EpisodeID, err)
			return SyncSessionResult{
				ID:             item.ID,
				Success:        false,
				ProgressSynced: false,
			}, false
		}
		mediaItemType = "podcastEpisode"
	} else {
		var mediaType string
		err := db.QueryRowContext(ctx, "SELECT mediaId, mediaType FROM libraryItems WHERE id = ? OR mediaId = ?", item.LibraryItemID, item.LibraryItemID).Scan(&mediaItemID, &mediaType)
		if err != nil {
			log.Warnf("[Sync Sessions] Library item not found for libraryItemId %s: %v", item.LibraryItemID, err)
			return SyncSessionResult{
				ID:             item.ID,
				Success:        false,
				ProgressSynced: false,
			}, false
		}
		if mediaType != "book" {
			log.Warnf("[Sync Sessions] Library item is not a book: %s (type %s)", item.LibraryItemID, mediaType)
			return SyncSessionResult{
				ID:             item.ID,
				Success:        false,
				ProgressSynced: false,
			}, false
		}
		mediaItemType = "book"
	}

	startedAtTime := parseJSONTime(item.StartedAt)
	updatedAtTime := parseJSONTime(item.UpdatedAt)

	startedAtStr := idb.TimeToDBStr(startedAtTime)
	updatedAtStr := idb.TimeToDBStr(updatedAtTime)

	extra := map[string]interface{}{
		"playMethod":    stringifyPlayMethod(item.PlayMethod),
		"deviceInfo":    stringifyDeviceInfo(item.DeviceInfo),
		"timeListened":  item.TimeListening,
		"lastTime":      item.CurrentTime,
		"libraryItemId": item.LibraryItemID,
	}
	extraBytes, _ := json.Marshal(extra)

	_, err := db.ExecContext(ctx, `
		INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, libraryId, extraData, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			userId = excluded.userId,
			mediaItemId = excluded.mediaItemId,
			mediaItemType = excluded.mediaItemType,
			startTime = excluded.startTime,
			libraryId = excluded.libraryId,
			extraData = excluded.extraData,
			createdAt = excluded.createdAt,
			updatedAt = excluded.updatedAt
	`, item.ID, userSess.ID, mediaItemID, mediaItemType, item.StartTime, item.LibraryID, string(extraBytes), startedAtStr, updatedAtStr)

	if err != nil {
		log.Errorf("[Sync Sessions] Database error inserting playback session: %v", err)
		return SyncSessionResult{
			ID:             item.ID,
			Success:        false,
			ProgressSynced: false,
		}, false
	}

	if isocket.GlobalAuth != nil {
		isocket.GlobalAuth.BroadcastPlaybackSessionAdded(userSess.ID, item.ID)
	}

	var progressID string
	var dbUpdatedAtStr sql.NullString
	errProg := db.QueryRowContext(ctx, `SELECT id, updatedAt FROM mediaProgresses WHERE userId = ? AND mediaItemId = ?`, userSess.ID, mediaItemID).Scan(&progressID, &dbUpdatedAtStr)
	progExists := (errProg == nil)

	progressUpdated := false
	if !progExists || (dbUpdatedAtStr.Valid && idb.ParseTimeStr(dbUpdatedAtStr.String) < updatedAtTime.UnixMilli()) {
		currentTimeVal := item.CurrentTime
		durationVal := item.Duration
		if durationVal <= 0 {
			durationVal = 0.0
		}

		isFinishedVal := false
		if durationVal > 0 && (durationVal-currentTimeVal < 10.0) {
			isFinishedVal = true
		}

		progPct := 0.0
		if durationVal > 0 {
			progPct = currentTimeVal / durationVal
			if progPct > 1.0 {
				progPct = 1.0
			}
		}

		progExtra := map[string]interface{}{
			"libraryItemId": item.LibraryItemID,
			"progress":      progPct,
		}
		progExtraBytes, _ := json.Marshal(progExtra)

		finishedAtNullable := interface{}(nil)
		if isFinishedVal {
			finishedAtNullable = idb.TimeToDBStr(updatedAtTime)
		}

		if progExists {
			_, err = db.ExecContext(ctx, `UPDATE mediaProgresses SET duration = ?, currentTime = ?, isFinished = ?, finishedAt = ?, extraData = ?, updatedAt = ? WHERE id = ?`,
				durationVal, currentTimeVal, func() int {
					if isFinishedVal {
						return 1
					}
					return 0
				}(), finishedAtNullable, string(progExtraBytes), updatedAtStr, progressID)
		} else {
			progressID = uuid.New().String()
			_, err = db.ExecContext(ctx, `INSERT INTO mediaProgresses (id, userId, mediaItemId, mediaItemType, duration, currentTime, isFinished, hideFromContinueListening, finishedAt, extraData, podcastId, createdAt, updatedAt) 
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				progressID, userSess.ID, mediaItemID, mediaItemType, durationVal, currentTimeVal, func() int {
					if isFinishedVal {
						return 1
					}
					return 0
				}(), 0, finishedAtNullable, string(progExtraBytes), podcastID, startedAtStr, updatedAtStr)
		}

		if err == nil {
			progressUpdated = true
		} else {
			log.Errorf("[Sync Sessions] Failed to sync media progress: %v", err)
		}
	}

	return SyncSessionResult{
		ID:             item.ID,
		Success:        true,
		ProgressSynced: progressUpdated,
	}, progressUpdated
}

// handleSyncLocalSession handles POST /api/session/local
func handleSyncLocalSession(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var item LocalSessionItem
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			log.Warnf("[Sync Session] Decode failed: %v", err)
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		res, progUpdated := syncSingleSession(ctx, db, userSess, item)

		if progUpdated {
			user, err := idb.GetUserFullByID(ctx, db, userSess.ID)
			if err == nil && user != nil {
				userJSON := user.ToOldJSONForBrowser(user.Type != "root")
				if isocket.GlobalAuth != nil {
					isocket.GlobalAuth.BroadcastToUser(userSess.ID, "user_updated", userJSON)
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(res)
	}
}

// handleSyncLocalSessions handles POST /api/session/local-all
func handleSyncLocalSessions(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var payload LocalSessionsPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			log.Warnf("[Sync Sessions] Decode failed: %v", err)
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		results := make([]SyncSessionResult, 0, len(payload.Sessions))
		progressUpdatedAny := false

		for _, item := range payload.Sessions {
			res, progUpdated := syncSingleSession(ctx, db, userSess, item)
			if progUpdated {
				progressUpdatedAny = true
			}
			results = append(results, res)
		}

		if progressUpdatedAny {
			user, err := idb.GetUserFullByID(ctx, db, userSess.ID)
			if err == nil && user != nil {
				userJSON := user.ToOldJSONForBrowser(user.Type != "root")
				if isocket.GlobalAuth != nil {
					isocket.GlobalAuth.BroadcastToUser(userSess.ID, "user_updated", userJSON)
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(SyncSessionsResponse{
			Results: results,
		})
	}
}
