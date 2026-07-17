package socket

import (
	"encoding/json"
	"fmt"

	log "audiobookshelf/internal/logger"
)

func (sa *Authority) getPlaybackSessionByID(sessionID string) (map[string]interface{}, error) {
	sa.mu.RLock()
	dbConn := sa.database
	sa.mu.RUnlock()
	if dbConn == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT ps.id, ps.userId, u.username, ps.mediaItemId, ps.mediaItemType, ps.startTime, 
		       COALESCE(ps.updatedAt, ps.createdAt, '') as updatedAt, COALESCE(ps.extraData, '') as extraData,
		       CASE 
		           WHEN ps.mediaItemType = 'podcastEpisode' THEN COALESCE(pe.title, '')
		           WHEN ps.mediaItemType = 'podcast' THEN COALESCE(p.title, '')
		           ELSE COALESCE(b.title, '')
		       END as title,
		       CASE 
		           WHEN ps.mediaItemType = 'podcastEpisode' THEN COALESCE(p.author, '')
		           WHEN ps.mediaItemType = 'podcast' THEN COALESCE(p.author, '')
		           ELSE COALESCE(li.authorNamesFirstLast, '')
		       END as author
		FROM playbackSessions ps
		LEFT JOIN users u ON u.id = ps.userId
		LEFT JOIN books b ON b.id = ps.mediaItemId AND ps.mediaItemType = 'book'
		LEFT JOIN libraryItems li ON li.mediaId = ps.mediaItemId AND li.mediaType = 'book' AND ps.mediaItemType = 'book'
		LEFT JOIN podcastEpisodes pe ON pe.id = ps.mediaItemId AND ps.mediaItemType = 'podcastEpisode'
		LEFT JOIN podcasts p ON (p.id = pe.podcastId AND ps.mediaItemType = 'podcastEpisode') OR (p.id = ps.mediaItemId AND ps.mediaItemType = 'podcast')
		WHERE ps.id = ?
	`
	var id, uID, username, mediaItemId, mediaItemType, updatedAt, extraDataStr, title, author string
	var startTime float64

	err := dbConn.QueryRow(query, sessionID).Scan(
		&id, &uID, &username, &mediaItemId, &mediaItemType, &startTime, &updatedAt, &extraDataStr, &title, &author,
	)
	if err != nil {
		return nil, err
	}

	playMethod := "HLS"
	deviceInfo := "Web Client"
	timeListened := 0.0
	lastTime := 0.0

	if extraDataStr != "" {
		var extra map[string]interface{}
		if err := json.Unmarshal([]byte(extraDataStr), &extra); err == nil {
			if val, ok := extra["playMethod"]; ok {
				if s, ok2 := val.(string); ok2 {
					playMethod = s
				}
			}
			if val, ok := extra["deviceInfo"]; ok {
				if s, ok2 := val.(string); ok2 {
					deviceInfo = s
				}
			}
			if val, ok := extra["timeListened"]; ok {
				if f, ok2 := val.(float64); ok2 {
					timeListened = f
				}
			}
			if val, ok := extra["lastTime"]; ok {
				if f, ok2 := val.(float64); ok2 {
					lastTime = f
				}
			}
		}
	}

	return map[string]interface{}{
		"id":            id,
		"userId":        uID,
		"username":      username,
		"mediaItemId":   mediaItemId,
		"mediaItemType": mediaItemType,
		"title":         title,
		"author":        author,
		"startTime":     startTime,
		"timeListened":  timeListened,
		"lastTime":      lastTime,
		"updatedAt":     updatedAt,
		"playMethod":    playMethod,
		"deviceInfo":    deviceInfo,
	}, nil
}

func (sa *Authority) BroadcastPlaybackSessionAdded(userID string, sessionID string) {
	sess, err := sa.getPlaybackSessionByID(sessionID)
	if err != nil {
		log.Printf("[Socket] Failed to retrieve playback session %s: %v", sessionID, err)
		return
	}
	sa.ClientEmitter(userID, "playback_session_added", sess)
	sa.AdminEmitter("playback_session_added", sess)
}

func (sa *Authority) BroadcastPlaybackSessionUpdated(userID string, sessionID string) {
	sess, err := sa.getPlaybackSessionByID(sessionID)
	if err != nil {
		log.Printf("[Socket] Failed to retrieve playback session %s: %v", sessionID, err)
		return
	}
	sa.ClientEmitter(userID, "playback_session_updated", sess)
	sa.AdminEmitter("playback_session_updated", sess)
}

func (sa *Authority) BroadcastPlaybackSessionRemoved(userID string, sessionID string) {
	payload := map[string]interface{}{
		"id":     sessionID,
		"userId": userID,
	}
	sa.ClientEmitter(userID, "playback_session_removed", payload)
	sa.AdminEmitter("playback_session_removed", payload)
}
