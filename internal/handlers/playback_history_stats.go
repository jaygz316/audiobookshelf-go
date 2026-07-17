package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	idb "audiobookshelf/internal/db"
)

func getListeningStatsInternal(dbConn *sql.DB, targetUserID string, isServer bool) (totalTime float64, todayTime float64, daysMap map[string]float64, dayOfWeekMap map[string]float64, itemsMap map[string]ListeningStatsItem, authorsMap map[string]float64, genresMap map[string]float64, topUsersMap map[string]float64, recentSessions []PlaybackSessionResponse, err error) {
	var query string
	var args []interface{}
	if isServer {
		query = listeningStatsQuery + ` ORDER BY COALESCE(ps.updatedAt, ps.createdAt) DESC`
	} else {
		query = listeningStatsQuery + ` WHERE ps.userId = ? ORDER BY COALESCE(ps.updatedAt, ps.createdAt) DESC`
		args = append(args, targetUserID)
	}

	rows, err := dbConn.Query(query, args...)
	if err != nil {
		return 0, 0, nil, nil, nil, nil, nil, nil, nil, err
	}
	defer rows.Close()

	daysMap = make(map[string]float64)
	dayOfWeekMap = make(map[string]float64)
	for i := 0; i <= 6; i++ {
		dayOfWeekMap[strconv.Itoa(i)] = 0.0
	}
	itemsMap = make(map[string]ListeningStatsItem)
	authorsMap = make(map[string]float64)
	genresMap = make(map[string]float64)
	topUsersMap = make(map[string]float64)
	recentSessions = make([]PlaybackSessionResponse, 0)

	todayStr := time.Now().UTC().Format("2006-01-02")

	for rows.Next() {
		var id, userId, username, mediaItemId, mediaItemType, updatedAt, extraDataStr, title, author, genresStr string
		var startTime float64

		err := rows.Scan(
			&id, &userId, &username, &mediaItemId, &mediaItemType, &startTime, &updatedAt, &extraDataStr, &title, &author, &genresStr,
		)
		if err != nil {
			log.Errorf("[Listening Stats] Failed to scan row: %v", err)
			continue
		}

		playMethod, deviceInfo, timeListened, lastTime := parsePlaybackSessionExtraData(extraDataStr)

		sessionResp := PlaybackSessionResponse{
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
		}

		if timeListened > 0 {
			totalTime += timeListened

			// Parse updatedAt to extract date and day of week
			if t, err := idb.ParseSQLiteTime(updatedAt); err == nil {
				utcTime := t.UTC()
				dateStr := utcTime.Format("2006-01-02")
				daysMap[dateStr] = daysMap[dateStr] + timeListened

				dayOfWeekKey := strconv.Itoa(int(utcTime.Weekday()))
				dayOfWeekMap[dayOfWeekKey] = dayOfWeekMap[dayOfWeekKey] + timeListened

				if dateStr == todayStr {
					todayTime += timeListened
				}
			}

			// Aggregate by item
			if item, exists := itemsMap[mediaItemId]; exists {
				item.TimeListened += timeListened
				itemsMap[mediaItemId] = item
			} else {
				itemsMap[mediaItemId] = ListeningStatsItem{
					TimeListened:  timeListened,
					Title:         title,
					Author:        author,
					MediaItemType: mediaItemType,
				}
			}

			// Aggregate by author
			if author != "" {
				parts := strings.Split(author, ",")
				for _, part := range parts {
					trimmed := strings.TrimSpace(part)
					if trimmed != "" {
						authorsMap[trimmed] = authorsMap[trimmed] + timeListened
					}
				}
			}

			// Aggregate by genre
			var genres []string
			if genresStr != "" && genresStr != "[]" {
				_ = json.Unmarshal([]byte(genresStr), &genres)
			}
			for _, g := range genres {
				trimmed := strings.TrimSpace(g)
				if trimmed != "" {
					genresMap[trimmed] = genresMap[trimmed] + timeListened
				}
			}

			// Aggregate by user (server-wide)
			if username != "" {
				topUsersMap[username] = topUsersMap[username] + timeListened
			} else if userId != "" {
				topUsersMap[userId] = topUsersMap[userId] + timeListened
			}
		}

		// Keep up to 10 recent sessions
		if len(recentSessions) < 10 {
			recentSessions = append(recentSessions, sessionResp)
		}
	}

	return totalTime, todayTime, daysMap, dayOfWeekMap, itemsMap, authorsMap, genresMap, topUsersMap, recentSessions, nil
}
