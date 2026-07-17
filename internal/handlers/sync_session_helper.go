package handlers

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/google/uuid"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	log "audiobookshelf/internal/logger"
	isocket "audiobookshelf/internal/socket"
)

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
