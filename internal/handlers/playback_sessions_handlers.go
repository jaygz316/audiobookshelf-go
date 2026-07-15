package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"

	"audiobookshelf/internal/core"
	isocket "audiobookshelf/internal/socket"
)

// handleGetPlaybackSessions retrieves the list of playback sessions joined with users and media details.
func handleGetPlaybackSessions(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)
		if user.Type != "root" && user.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		userIdFilter := r.URL.Query().Get("userId")

		query := `
			SELECT
				ps.id,
				ps.userId,
				COALESCE(u.username, '') as username,
				ps.mediaItemId,
				ps.mediaItemType,
				ps.startTime,
				COALESCE(ps.updatedAt, ps.createdAt, '') as updatedAt,
				COALESCE(ps.extraData, '') as extraData,
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
		var args []interface{}
		if userIdFilter != "" {
			query += " WHERE ps.userId = ?"
			args = append(args, userIdFilter)
		}
		query += " ORDER BY COALESCE(ps.updatedAt, ps.createdAt) DESC"

		rows, err := db.Query(query, args...)
		if err != nil {
			log.Errorf("[Playback Sessions] Failed to query playback sessions: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

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

		sessions := make([]PlaybackSessionResponse, 0)
		for rows.Next() {
			var id, userId, username, mediaItemId, mediaItemType, updatedAt, extraDataStr, title, author string
			var startTime float64

			err := rows.Scan(
				&id, &userId, &username, &mediaItemId, &mediaItemType, &startTime, &updatedAt, &extraDataStr, &title, &author,
			)
			if err != nil {
				log.Errorf("[Playback Sessions] Failed to scan row: %v", err)
				http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
				return
			}

			// Parse extraData
			playMethod := "HLS"
			deviceInfo := "Web Client"
			timeListened := 0.0
			lastTime := 0.0

			if extraDataStr != "" {
				var extra struct {
					PlayMethod   *string  `json:"playMethod"`
					DeviceInfo   *string  `json:"deviceInfo"`
					TimeListened *float64 `json:"timeListened"`
					LastTime     *float64 `json:"lastTime"`
				}
				if err := json.Unmarshal([]byte(extraDataStr), &extra); err == nil {
					if extra.PlayMethod != nil {
						playMethod = *extra.PlayMethod
					}
					if extra.DeviceInfo != nil {
						deviceInfo = *extra.DeviceInfo
					}
					if extra.TimeListened != nil {
						timeListened = *extra.TimeListened
					}
					if extra.LastTime != nil {
						lastTime = *extra.LastTime
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

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"sessions": sessions,
		})
	}
}

// handleClosePlaybackSession closes a playback session and broadcasts the removal.
func handleClosePlaybackSession(db *sql.DB, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// Retrieve session first to know the owner
		var userID string
		err := db.QueryRowContext(r.Context(), "SELECT userId FROM playbackSessions WHERE id = ?", sessionID).Scan(&userID)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, `{"error": "Session Not Found"}`, http.StatusNotFound)
				return
			}
			http.Error(w, `{"error": "Database Error"}`, http.StatusInternalServerError)
			return
		}

		// Check permissions: only owner or admin can close it
		if userID != userSess.ID && userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		// Delete session
		_, err = db.ExecContext(r.Context(), "DELETE FROM playbackSessions WHERE id = ?", sessionID)
		if err != nil {
			log.Errorf("[Close Playback Session] Delete failed: %v", err)
			http.Error(w, `{"error": "Database Error"}`, http.StatusInternalServerError)
			return
		}

		// Broadcast removal to socket
		if isocket.GlobalAuth != nil {
			isocket.GlobalAuth.BroadcastPlaybackSessionRemoved(userID, sessionID)
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
