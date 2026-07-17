package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
)

func handleGetUserListeningSessions(dbConn *sql.DB, targetUserID string, page int, itemsPerPage int) (map[string]interface{}, error) {
	// Query total count
	var total int
	var err error
	if targetUserID == "" {
		err = dbConn.QueryRow("SELECT COUNT(*) FROM playbackSessions").Scan(&total)
	} else {
		err = dbConn.QueryRow("SELECT COUNT(*) FROM playbackSessions WHERE userId = ?", targetUserID).Scan(&total)
	}
	if err != nil {
		return nil, err
	}

	var query string
	var args []interface{}
	if targetUserID == "" {
		query = listeningSessionsQuery + ` ORDER BY COALESCE(ps.updatedAt, ps.createdAt) DESC LIMIT ? OFFSET ?`
		args = append(args, itemsPerPage, page*itemsPerPage)
	} else {
		query = listeningSessionsQuery + ` WHERE ps.userId = ? ORDER BY COALESCE(ps.updatedAt, ps.createdAt) DESC LIMIT ? OFFSET ?`
		args = append(args, targetUserID, itemsPerPage, page*itemsPerPage)
	}

	rows, err := dbConn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := make([]PlaybackSessionResponse, 0)
	for rows.Next() {
		var id, userId, username, mediaItemId, mediaItemType, updatedAt, extraDataStr, title, author string
		var startTime float64

		err := rows.Scan(
			&id, &userId, &username, &mediaItemId, &mediaItemType, &startTime, &updatedAt, &extraDataStr, &title, &author,
		)
		if err != nil {
			log.Errorf("[Listening Sessions] Failed to scan row: %v", err)
			continue
		}

		playMethod, deviceInfo, timeListened, lastTime := parsePlaybackSessionExtraData(extraDataStr)

		sessions = append(sessions, PlaybackSessionResponse{
			ID:            id,
			UserID:        userId,
			Username:      username,
			MediaItemID:   mediaItemId,
			MediaItemType: mediaItemType,
			Title:         title,
			Author:        author,
			StartTime:     startTime,
			TimeListened:  timeListened,
			LastTime:      lastTime,
			UpdatedAt:     updatedAt,
			PlayMethod:    playMethod,
			DeviceInfo:    deviceInfo,
		})
	}

	return map[string]interface{}{
		"sessions":     sessions,
		"total":        total,
		"page":         page,
		"itemsPerPage": itemsPerPage,
	}, nil
}
