package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
)

func parsePlaybackSessionExtraData(extraDataStr string) (playMethod string, deviceInfo string, timeListened float64, lastTime float64) {
	playMethod = "HLS"
	deviceInfo = "Web Client"
	timeListened = 0.0
	lastTime = 0.0

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
	return
}

func getUserListeningStats(dbConn *sql.DB, targetUserID string) (ListeningStatsResponse, error) {
	totalTime, todayTime, daysMap, dayOfWeekMap, itemsMap, authorsMap, genresMap, _, recentSessions, err := getListeningStatsInternal(dbConn, targetUserID, false)
	if err != nil {
		return ListeningStatsResponse{}, err
	}
	var itemsFinished int
	err = dbConn.QueryRow("SELECT COUNT(*) FROM mediaProgresses WHERE userId = ? AND isFinished = 1", targetUserID).Scan(&itemsFinished)
	if err != nil {
		log.Errorf("failed to count finished items for user: %v", err)
	}
	return ListeningStatsResponse{
		TotalTime:      totalTime,
		Today:          todayTime,
		Days:           daysMap,
		DayOfWeek:      dayOfWeekMap,
		Items:          itemsMap,
		TopAuthors:     authorsMap,
		TopGenres:      genresMap,
		RecentSessions: recentSessions,
		ItemsFinished:  itemsFinished,
		DaysListened:   len(daysMap),
	}, nil
}

func getServerListeningStats(dbConn *sql.DB) (ServerListeningStatsResponse, error) {
	totalTime, todayTime, daysMap, dayOfWeekMap, itemsMap, authorsMap, genresMap, topUsersMap, recentSessions, err := getListeningStatsInternal(dbConn, "", true)
	if err != nil {
		return ServerListeningStatsResponse{}, err
	}
	var itemsFinished int
	err = dbConn.QueryRow("SELECT COUNT(*) FROM mediaProgresses WHERE isFinished = 1").Scan(&itemsFinished)
	if err != nil {
		log.Errorf("failed to count finished items for server: %v", err)
	}
	return ServerListeningStatsResponse{
		TotalTime:      totalTime,
		Today:          todayTime,
		Days:           daysMap,
		DayOfWeek:      dayOfWeekMap,
		Items:          itemsMap,
		TopAuthors:     authorsMap,
		TopGenres:      genresMap,
		TopUsers:       topUsersMap,
		RecentSessions: recentSessions,
		ItemsFinished:  itemsFinished,
		DaysListened:   len(daysMap),
	}, nil
}
