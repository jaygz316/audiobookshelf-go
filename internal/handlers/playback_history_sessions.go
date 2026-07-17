package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"strings"
)

func handleGetUserListeningSessions(dbConn *sql.DB, targetUserID string, page int, itemsPerPage int, mediaItemID string, libraryItemID string) (map[string]interface{}, error) {
	// Query total count
	var total int
	var err error

	countQuery := "SELECT COUNT(*) FROM playbackSessions ps"
	var countArgs []interface{}
	var countWheres []string

	if targetUserID != "" {
		countWheres = append(countWheres, "ps.userId = ?")
		countArgs = append(countArgs, targetUserID)
	}

	if libraryItemID != "" {
		countWheres = append(countWheres, "(ps.mediaItemId = ? OR json_extract(ps.extraData, '$.libraryItemId') = ?)")
		countArgs = append(countArgs, libraryItemID, libraryItemID)
	} else if mediaItemID != "" {
		countWheres = append(countWheres, "ps.mediaItemId = ?")
		countArgs = append(countArgs, mediaItemID)
	}

	if len(countWheres) > 0 {
		countQuery += " WHERE " + strings.Join(countWheres, " AND ")
	}

	err = dbConn.QueryRow(countQuery, countArgs...).Scan(&total)
	if err != nil {
		return nil, err
	}

	// Base SELECT query
	var query string
	var args []interface{}

	query = listeningSessionsQuery

	var whereClauses []string
	if targetUserID != "" {
		whereClauses = append(whereClauses, "ps.userId = ?")
		args = append(args, targetUserID)
	}

	if libraryItemID != "" {
		whereClauses = append(whereClauses, "(ps.mediaItemId = ? OR json_extract(ps.extraData, '$.libraryItemId') = ?)")
		args = append(args, libraryItemID, libraryItemID)
	} else if mediaItemID != "" {
		whereClauses = append(whereClauses, "ps.mediaItemId = ?")
		args = append(args, mediaItemID)
	}

	if len(whereClauses) > 0 {
		query += " WHERE " + strings.Join(whereClauses, " AND ")
	}

	query += ` ORDER BY COALESCE(ps.updatedAt, ps.createdAt) DESC LIMIT ? OFFSET ?`
	args = append(args, itemsPerPage, page*itemsPerPage)

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
