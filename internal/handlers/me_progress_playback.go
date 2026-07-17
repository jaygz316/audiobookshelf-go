package handlers

import (
	"context"
	"database/sql"
	"encoding/json"

	log "audiobookshelf/internal/logger"
	isocket "audiobookshelf/internal/socket"
	"github.com/google/uuid"
)

// updateOrCreatePlaybackSession updates an active playback session if currentTime changed, or creates a fallback session if none exists.
func updateOrCreatePlaybackSession(ctx context.Context, db *sql.DB, userID, mediaItemID, mediaItemType, libraryItemID string, currentTimeVal float64, nowStr string) error {
	var sessID string
	var sessExtraStr sql.NullString
	errSess := db.QueryRowContext(ctx, `SELECT id, extraData FROM playbackSessions WHERE userId = ? AND mediaItemId = ? ORDER BY COALESCE(updatedAt, createdAt) DESC LIMIT 1`, userID, mediaItemID).Scan(&sessID, &sessExtraStr)
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
		_, errSessUpdate := db.ExecContext(ctx, `UPDATE playbackSessions SET extraData = ?, updatedAt = ? WHERE id = ?`, string(sessExtraBytes), nowStr, sessID)
		if errSessUpdate != nil {
			log.Errorf("[Me Progress] Failed to update playback session: %v", errSessUpdate)
			return errSessUpdate
		} else if isocket.GlobalAuth != nil {
			isocket.GlobalAuth.BroadcastPlaybackSessionUpdated(userID, sessID)
		}
	} else if errSess == sql.ErrNoRows {
		// Fallback: Create playback session if none exists
		var resolvedLibraryID sql.NullString
		if mediaItemType == "podcastEpisode" {
			var podcastID string
			_ = db.QueryRowContext(ctx, "SELECT podcastId FROM podcastEpisodes WHERE id = ?", mediaItemID).Scan(&podcastID)
			if podcastID != "" {
				_ = db.QueryRowContext(ctx, "SELECT libraryId FROM libraryItems WHERE mediaId = ? AND mediaType = 'podcast'", podcastID).Scan(&resolvedLibraryID)
			}
		} else {
			_ = db.QueryRowContext(ctx, "SELECT libraryId FROM libraryItems WHERE mediaId = ? AND mediaType = 'book'", mediaItemID).Scan(&resolvedLibraryID)
		}
		sessID = uuid.New().String()
		sessExtra := map[string]interface{}{
			"libraryItemId": libraryItemID,
			"timeListened":  0.0,
			"lastTime":      currentTimeVal,
			"playMethod":    "HLS",
			"deviceInfo":    "Web Client",
		}
		sessExtraBytes, _ := json.Marshal(sessExtra)
		_, errSessInsert := db.ExecContext(ctx, `INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, libraryId, extraData, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			sessID, userID, mediaItemID, mediaItemType, currentTimeVal, resolvedLibraryID, string(sessExtraBytes), nowStr, nowStr)
		if errSessInsert != nil {
			log.Errorf("[Me Progress] Failed to create fallback playback session: %v", errSessInsert)
			return errSessInsert
		} else if isocket.GlobalAuth != nil {
			isocket.GlobalAuth.BroadcastPlaybackSessionAdded(userID, sessID)
		}
	} else {
		return errSess
	}
	return nil
}
