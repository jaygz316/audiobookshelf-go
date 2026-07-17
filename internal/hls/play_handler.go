package hls

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
	log "audiobookshelf/internal/logger"
	isocket "audiobookshelf/internal/socket"
)

// HandlePlayItem returns an HTTP handler for creating a playback session.
func HandlePlayItem(db *sql.DB, sm *StreamManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "Unauthorized"}`))
			return
		}
		user := userVal.(*core.UserSession)

		// Get item ID from request path.
		parts := strings.Split(r.URL.Path, "/")
		var itemID string
		for i, part := range parts {
			if part == "items" && i+1 < len(parts) {
				itemID = parts[i+1]
				break
			}
		}

		var episodeID string
		for i, part := range parts {
			if part == "play" && i+1 < len(parts) {
				episodeID = parts[i+1]
				break
			}
		}

		if itemID == "" || strings.Contains(itemID, "..") || strings.Contains(itemID, "/") || strings.Contains(itemID, "\\") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": "Invalid Item ID"}`))
			return
		}

		if episodeID != "" && (strings.Contains(episodeID, "..") || strings.Contains(episodeID, "/") || strings.Contains(episodeID, "\\")) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": "Invalid Episode ID"}`))
			return
		}

		type PlayRequest struct {
			StartTime float64 `json:"startTime"`
		}
		var playReq PlayRequest
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&playReq)
		}

		sessionID, mediaItemID, mediaItemType, _, err := resolveMediaItemAndCreateSession(
			r.Context(), db, itemID, episodeID, user.ID, playReq.StartTime,
		)
		if err != nil {
			log.Printf("[HandlePlayItem] Failed: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf(`{"error": "Failed to create playback session: %v"}`, err)))
			return
		}

		if isocket.GlobalAuth != nil {
			isocket.GlobalAuth.BroadcastPlaybackSessionAdded(user.ID, sessionID)
		}

		var audioTracks []map[string]interface{}
		var displayTitle string
		var displayAuthor string

		if mediaItemType == "podcastEpisode" {
			audioTracks, displayTitle, displayAuthor, err = getPodcastEpisodeTracks(r.Context(), db, mediaItemID, sessionID)
		} else {
			audioTracks, displayTitle, displayAuthor, err = getBookTracks(r.Context(), db, mediaItemID, sessionID)
		}

		if err != nil {
			log.Printf("[HandlePlayItem] Track loading error: %v", err)
		}

		resp := map[string]interface{}{
			"id":                sessionID,
			"currentTime":       playReq.StartTime,
			"displayTitle":      displayTitle,
			"displayAuthor":     displayAuthor,
			"playMethod":        2, // PlayMethod.TRANSCODE
			"audioTracks":       audioTracks,
			"clientPlaylistUri": fmt.Sprintf("/hls/%s/output.m3u8", sessionID),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}
}
