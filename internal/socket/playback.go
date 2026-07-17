package socket

import (
	"encoding/json"

	log "audiobookshelf/internal/logger"
)

func (sa *Authority) getPlaybackSessionsForUser(userID string) []interface{} {
	sa.mu.RLock()
	dbConn := sa.database
	sa.mu.RUnlock()
	if dbConn == nil {
		return []interface{}{}
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
		WHERE ps.userId = ?
		ORDER BY COALESCE(ps.updatedAt, ps.createdAt) DESC
	`
	rows, err := dbConn.Query(query, userID)
	if err != nil {
		log.Printf("[Socket] Failed to query playback sessions for user %s: %v", userID, err)
		return []interface{}{}
	}
	defer rows.Close()

	var sessions []interface{}
	for rows.Next() {
		var id, uID, username, mediaItemId, mediaItemType, updatedAt, extraDataStr, title, author string
		var startTime float64

		err := rows.Scan(
			&id, &uID, &username, &mediaItemId, &mediaItemType, &startTime, &updatedAt, &extraDataStr, &title, &author,
		)
		if err != nil {
			log.Printf("[Socket] Failed to scan playback session: %v", err)
			continue
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

		sessions = append(sessions, map[string]interface{}{
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
		})
	}
	return sessions
}
