package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	log "audiobookshelf/internal/logger"
	isocket "audiobookshelf/internal/socket"
)

// syncSingleProgress processes a single local progress item, updates the database,
// and broadcasts session updates if necessary.
func syncSingleProgress(ctx context.Context, db *sql.DB, userSess *core.UserSession, item LocalMediaProgressItem, now time.Time) (bool, error) {
	var mediaItemID, mediaItemType string
	var podcastID sql.NullString

	if item.EpisodeID != nil && *item.EpisodeID != "" {
		err := db.QueryRowContext(ctx, "SELECT id, podcastId FROM podcastEpisodes WHERE id = ?", *item.EpisodeID).Scan(&mediaItemID, &podcastID)
		if err != nil {
			log.Warnf("[Sync Progress] Episode not found for episodeId %s: %v", *item.EpisodeID, err)
			return false, err
		}
		mediaItemType = "podcastEpisode"
	} else {
		var mediaType string
		err := db.QueryRowContext(ctx, "SELECT mediaId, mediaType FROM libraryItems WHERE id = ? OR mediaId = ?", item.LibraryItemID, item.LibraryItemID).Scan(&mediaItemID, &mediaType)
		if err != nil {
			log.Warnf("[Sync Progress] Library item not found for libraryItemId %s: %v", item.LibraryItemID, err)
			return false, err
		}
		if mediaType != "book" {
			log.Warnf("[Sync Progress] Library item is not a book: %s (type %s)", item.LibraryItemID, mediaType)
			return false, nil
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
			return false, nil
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
	nowStr := idb.TimeToDBStr(now)

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
		return false, err
	}

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

	return true, nil
}
