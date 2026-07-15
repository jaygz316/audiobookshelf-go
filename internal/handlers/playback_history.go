package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

type ListeningStatsItem struct {
	TimeListened  float64 `json:"timeListened"`
	Title         string  `json:"title"`
	Author        string  `json:"author"`
	MediaItemType string  `json:"mediaItemType"`
}

type PlaybackSessionResponse struct {
	ID            string  `json:"id"`
	UserID        string  `json:"userId"`
	Username      string  `json:"username"`
	MediaItemID   string  `json:"mediaItemId"`
	MediaItemType string  `json:"mediaItemType"`
	Title         string  `json:"title"`
	Author        string  `json:"author"`
	StartTime     float64 `json:"startTime"`
	TimeListened  float64 `json:"timeListened"`
	LastTime      float64 `json:"lastTime"`
	UpdatedAt     string  `json:"updatedAt"`
	PlayMethod    string  `json:"playMethod"`
	DeviceInfo    string  `json:"deviceInfo"`
}

type ListeningStatsResponse struct {
	TotalTime      float64                       `json:"totalTime"`
	Today          float64                       `json:"today"`
	Days           map[string]float64            `json:"days"`
	DayOfWeek      map[string]float64            `json:"dayOfWeek"`
	Items          map[string]ListeningStatsItem `json:"items"`
	TopAuthors     map[string]float64            `json:"topAuthors"`
	TopGenres      map[string]float64            `json:"topGenres"`
	RecentSessions []PlaybackSessionResponse     `json:"recentSessions"`
	ItemsFinished  int                           `json:"itemsFinished"`
	DaysListened   int                           `json:"daysListened"`
}

type ServerListeningStatsResponse struct {
	TotalTime      float64                       `json:"totalTime"`
	Today          float64                       `json:"today"`
	Days           map[string]float64            `json:"days"`
	DayOfWeek      map[string]float64            `json:"dayOfWeek"`
	Items          map[string]ListeningStatsItem `json:"items"`
	TopAuthors     map[string]float64            `json:"topAuthors"`
	TopGenres      map[string]float64            `json:"topGenres"`
	TopUsers       map[string]float64            `json:"topUsers"`
	RecentSessions []PlaybackSessionResponse     `json:"recentSessions"`
	ItemsFinished  int                           `json:"itemsFinished"`
	DaysListened   int                           `json:"daysListened"`
}

func getListeningStatsInternal(dbConn *sql.DB, targetUserID string, isServer bool) (totalTime float64, todayTime float64, daysMap map[string]float64, dayOfWeekMap map[string]float64, itemsMap map[string]ListeningStatsItem, authorsMap map[string]float64, genresMap map[string]float64, topUsersMap map[string]float64, recentSessions []PlaybackSessionResponse, err error) {
	baseQuery := `
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
		       END as author,
		       CASE 
		           WHEN ps.mediaItemType = 'podcastEpisode' THEN COALESCE(p.genres, '[]')
		           WHEN ps.mediaItemType = 'podcast' THEN COALESCE(p.genres, '[]')
		           ELSE COALESCE(b.genres, '[]')
		       END as genres
		FROM playbackSessions ps
		LEFT JOIN users u ON u.id = ps.userId
		LEFT JOIN books b ON b.id = ps.mediaItemId AND ps.mediaItemType = 'book'
		LEFT JOIN libraryItems li ON li.mediaId = ps.mediaItemId AND li.mediaType = 'book' AND ps.mediaItemType = 'book'
		LEFT JOIN podcastEpisodes pe ON pe.id = ps.mediaItemId AND ps.mediaItemType = 'podcastEpisode'
		LEFT JOIN podcasts p ON (p.id = pe.podcastId AND ps.mediaItemType = 'podcastEpisode') OR (p.id = ps.mediaItemId AND ps.mediaItemType = 'podcast')
	`

	var query string
	var args []interface{}
	if isServer {
		query = baseQuery + ` ORDER BY COALESCE(ps.updatedAt, ps.createdAt) DESC`
	} else {
		query = baseQuery + ` WHERE ps.userId = ? ORDER BY COALESCE(ps.updatedAt, ps.createdAt) DESC`
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

		// Parse extraData
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

	baseQuery := `
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
	`

	var query string
	var args []interface{}
	if targetUserID == "" {
		query = baseQuery + ` ORDER BY COALESCE(ps.updatedAt, ps.createdAt) DESC LIMIT ? OFFSET ?`
		args = append(args, itemsPerPage, page*itemsPerPage)
	} else {
		query = baseQuery + ` WHERE ps.userId = ? ORDER BY COALESCE(ps.updatedAt, ps.createdAt) DESC LIMIT ? OFFSET ?`
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

		// Parse extraData
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

// handleGetMeListeningStats handles GET /api/me/listening-stats
func handleGetMeListeningStats(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		stats, err := getUserListeningStats(db, userSess.ID)
		if err != nil {
			log.Errorf("[Listening Stats] Failed to query stats: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	}
}

// handleGetMeListeningSessions handles GET /api/me/listening-sessions
func handleGetMeListeningSessions(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		page := 0
		if pVal := r.URL.Query().Get("page"); pVal != "" {
			if p, err := strconv.Atoi(pVal); err == nil {
				page = p
			}
		}
		itemsPerPage := 10
		if limitVal := r.URL.Query().Get("itemsPerPage"); limitVal != "" {
			if limit, err := strconv.Atoi(limitVal); err == nil {
				itemsPerPage = limit
			}
		}

		sessions, err := handleGetUserListeningSessions(db, userSess.ID, page, itemsPerPage)
		if err != nil {
			log.Errorf("[Listening Sessions] Failed to query sessions: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sessions)
	}
}

// handleGetServerListeningStats handles GET /api/server-listening-stats
func handleGetServerListeningStats(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		stats, err := getServerListeningStats(db)
		if err != nil {
			log.Errorf("[Server Listening Stats] Failed to query stats: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	}
}

// handleGetServerListeningSessions handles GET /api/server-listening-sessions
func handleGetServerListeningSessions(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		page := 0
		if pVal := r.URL.Query().Get("page"); pVal != "" {
			if p, err := strconv.Atoi(pVal); err == nil {
				page = p
			}
		}
		itemsPerPage := 10
		if limitVal := r.URL.Query().Get("itemsPerPage"); limitVal != "" {
			if limit, err := strconv.Atoi(limitVal); err == nil {
				itemsPerPage = limit
			}
		}

		sessions, err := handleGetUserListeningSessions(db, "", page, itemsPerPage)
		if err != nil {
			log.Errorf("[Listening Sessions] Failed to query server sessions: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sessions)
	}
}
