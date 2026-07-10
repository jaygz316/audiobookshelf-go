package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
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
	RecentSessions []PlaybackSessionResponse     `json:"recentSessions"`
}

func getUserListeningStats(dbConn *sql.DB, targetUserID string) (ListeningStatsResponse, error) {
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
	rows, err := dbConn.Query(query, targetUserID)
	if err != nil {
		return ListeningStatsResponse{}, err
	}
	defer rows.Close()

	var totalTime float64
	var todayTime float64
	daysMap := make(map[string]float64)
	dayOfWeekMap := make(map[string]float64)
	// Initialize dayOfWeekMap keys to "0"..."6" to ensure they are represented in json
	for i := 0; i <= 6; i++ {
		dayOfWeekMap[strconv.Itoa(i)] = 0.0
	}
	itemsMap := make(map[string]ListeningStatsItem)
	recentSessions := make([]PlaybackSessionResponse, 0)

	todayStr := time.Now().UTC().Format("2006-01-02")

	for rows.Next() {
		var id, userId, username, mediaItemId, mediaItemType, updatedAt, extraDataStr, title, author string
		var startTime float64

		err := rows.Scan(
			&id, &userId, &username, &mediaItemId, &mediaItemType, &startTime, &updatedAt, &extraDataStr, &title, &author,
		)
		if err != nil {
			log.Printf("[Listening Stats] Failed to scan row: %v", err)
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

		// Aggregate stats if timeListened > 0
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
		}

		// Keep up to 10 recent sessions
		if len(recentSessions) < 10 {
			recentSessions = append(recentSessions, sessionResp)
		}
	}

	return ListeningStatsResponse{
		TotalTime:      totalTime,
		Today:          todayTime,
		Days:           daysMap,
		DayOfWeek:      dayOfWeekMap,
		Items:          itemsMap,
		RecentSessions: recentSessions,
	}, nil
}

func handleGetUserListeningSessions(dbConn *sql.DB, targetUserID string, page int, itemsPerPage int) (map[string]interface{}, error) {
	// Query total count
	var total int
	err := dbConn.QueryRow("SELECT COUNT(*) FROM playbackSessions WHERE userId = ?", targetUserID).Scan(&total)
	if err != nil {
		return nil, err
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
		LIMIT ? OFFSET ?
	`
	offset := page * itemsPerPage
	rows, err := dbConn.Query(query, targetUserID, itemsPerPage, offset)
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
			log.Printf("[Listening Sessions] Failed to scan row: %v", err)
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
			log.Printf("[Listening Stats] Failed to query stats: %v", err)
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
			log.Printf("[Listening Sessions] Failed to query sessions: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sessions)
	}
}
