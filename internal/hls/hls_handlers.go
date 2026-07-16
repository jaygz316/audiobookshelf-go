package hls

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"audiobookshelf/internal/core"
	log "audiobookshelf/internal/logger"
	isocket "audiobookshelf/internal/socket"
)

// ServeHLS returns an HTTP handler for intercepting HLS playlist and segment requests.
func ServeHLS(db *sql.DB, metadataPath string, sm *StreamManager, socketAuth *isocket.Authority) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[HLS Gateway] Request: %s %s", r.Method, r.URL.String())
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			log.Printf("[HLS Gateway] Auth missing in context")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		userSess, ok := userVal.(*core.UserSession)
		if !ok || userSess == nil {
			log.Printf("[HLS Gateway] Invalid user session in context")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		path := r.URL.Path
		parts := strings.Split(strings.Trim(path, "/"), "/")
		hlsIdx := -1
		for i, part := range parts {
			if part == "hls" {
				hlsIdx = i
				break
			}
		}
		if hlsIdx == -1 || hlsIdx+2 >= len(parts) {
			log.Printf("[HLS Gateway] Bad Request: hls prefix not found or path too short")
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		streamID := parts[hlsIdx+1]
		fileName := parts[hlsIdx+2]

		if filepath.Base(fileName) != fileName {
			log.Printf("[HLS Gateway] Traversal attempt in fileName: %s", fileName)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		ext := filepath.Ext(fileName)
		log.Printf("[HLS Gateway] streamID: %s, fileName: %s, ext: %s", streamID, fileName, ext)
		if ext != ".ts" && ext != ".m3u8" && ext != ".mp4" && ext != ".m4s" {
			log.Printf("[HLS Gateway] Unsupported file format: %s", ext)
			http.Error(w, "Unsupported file format", http.StatusBadRequest)
			return
		}

		stream, err := sm.LoadOrCreateStream(db, streamID, metadataPath, socketAuth)
		if err != nil {
			log.Printf("[HLS Gateway] Error loading or creating stream %s: %v", streamID, err)
			http.Error(w, "Stream not found", http.StatusNotFound)
			return
		}

		if userSess.Type != "admin" && userSess.Type != "root" && stream.UserID != userSess.ID {
			log.Printf("[HLS Gateway] Forbidden: User %s (%s) does not own stream %s (owner: %s)", userSess.Username, userSess.Type, streamID, stream.UserID)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		fullFilePath := filepath.Join(stream.StreamPath, fileName)

		if _, err := os.Stat(fullFilePath); os.IsNotExist(err) {
			log.Printf("[HLS Gateway] File not found on disk: %s", fullFilePath)
			if ext == ".ts" {
				segNum := parseSegmentNumber(fileName)
				if segNum >= 0 {
					sTime, shouldReset := stream.CheckSegmentNumberRequest(segNum)
					if shouldReset {
						log.Printf("[HLS Gateway] Resetting stream %s at segment %d (time %.2fs)", streamID, segNum, sTime)
						_ = stream.Reset(sTime - (stream.SegmentLength * 5.0))
						emitWebsocketEvent(socketAuth, stream.UserID, "stream_reset", map[string]interface{}{
							"startTime": sTime,
							"streamId":  streamID,
						})
					}
				}

				// Wait for the segment to become ready (up to 10 seconds)
				ticker := time.NewTicker(100 * time.Millisecond)
				defer ticker.Stop()
				timeout := time.After(10 * time.Second)
				found := false
				for {
					select {
					case <-ticker.C:
						if _, err := os.Stat(fullFilePath); err == nil {
							found = true
							break
						}
					case <-timeout:
						break
					case <-r.Context().Done():
						log.Printf("[HLS Gateway] Request context cancelled for %s", fileName)
						return
					}
					if found {
						break
					}
				}
			}
		}

		if _, err := os.Stat(fullFilePath); os.IsNotExist(err) {
			log.Printf("[HLS Gateway] File still not found after wait: %s", fullFilePath)
			http.Error(w, "Segment not ready", http.StatusNotFound)
			return
		}

		log.Printf("[HLS Gateway] Serving file: %s", fullFilePath)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if ext == ".m3u8" {
			w.Header().Set("Content-Type", "application/x-mpegURL")
			content, err := os.ReadFile(fullFilePath)
			if err != nil {
				http.Error(w, "Playlist not found", http.StatusNotFound)
				return
			}
			token := r.URL.Query().Get("token")
			if token == "" {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					token = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}
			if token != "" {
				lines := strings.Split(string(content), "\n")
				for i, line := range lines {
					trimmed := strings.TrimSpace(line)
					if strings.HasSuffix(trimmed, ".ts") || strings.HasSuffix(trimmed, ".m4s") || strings.HasSuffix(trimmed, ".mp4") {
						if strings.Contains(trimmed, "?") {
							lines[i] = trimmed + "&token=" + token
						} else {
							lines[i] = trimmed + "?token=" + token
						}
					}
				}
				content = []byte(strings.Join(lines, "\n"))
			}
			_, _ = w.Write(content)
			return
		} else if ext == ".ts" {
			w.Header().Set("Content-Type", "video/MP2T")
		}
		http.ServeFile(w, r, fullFilePath)
	}
}

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

		var mediaItemID string = itemID
		var mediaItemType string = "book"
		var resolvedLibraryID sql.NullString

		// Check if itemID exists in libraryItems
		var liMediaID, liMediaType, liLibraryID string
		err := db.QueryRowContext(r.Context(), "SELECT mediaId, mediaType, libraryId FROM libraryItems WHERE id = ?", itemID).Scan(&liMediaID, &liMediaType, &liLibraryID)
		if err == nil {
			resolvedLibraryID.Valid = true
			resolvedLibraryID.String = liLibraryID
			if liMediaType == "book" {
				mediaItemID = liMediaID
				mediaItemType = "book"
			} else if liMediaType == "podcast" {
				if episodeID != "" {
					mediaItemID = episodeID
					mediaItemType = "podcastEpisode"
				} else {
					// If a podcast, get the first episode
					var epID string
					errEp := db.QueryRowContext(r.Context(), "SELECT id FROM podcastEpisodes WHERE podcastId = ? LIMIT 1", liMediaID).Scan(&epID)
					if errEp == nil {
						mediaItemID = epID
						mediaItemType = "podcastEpisode"
					} else {
						mediaItemID = liMediaID
						mediaItemType = "podcast"
					}
				}
			}
		} else {
			// Not in libraryItems directly. Check if it's a book ID in books
			var bookExists int
			errBook := db.QueryRowContext(r.Context(), "SELECT 1 FROM books WHERE id = ?", itemID).Scan(&bookExists)
			if errBook == nil && bookExists == 1 {
				mediaItemID = itemID
				mediaItemType = "book"
				_ = db.QueryRowContext(r.Context(), "SELECT libraryId FROM libraryItems WHERE mediaId = ? AND mediaType = 'book'", itemID).Scan(&resolvedLibraryID)
			} else {
				// Check if it's a podcastEpisode ID in podcastEpisodes
				var podcastID string
				errEp := db.QueryRowContext(r.Context(), "SELECT podcastId FROM podcastEpisodes WHERE id = ?", itemID).Scan(&podcastID)
				if errEp == nil {
					mediaItemID = itemID
					mediaItemType = "podcastEpisode"
					_ = db.QueryRowContext(r.Context(), "SELECT libraryId FROM libraryItems WHERE mediaId = ? AND mediaType = 'podcast'", podcastID).Scan(&resolvedLibraryID)
				}
			}
		}

		sessionID := uuid.New().String()
		_, _ = db.ExecContext(r.Context(), "DELETE FROM playbackSessions WHERE userId = ? AND mediaItemId = ?", user.ID, mediaItemID)

		extraData := fmt.Sprintf(`{"libraryItemId": %q}`, itemID)
		query := `INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, libraryId, extraData, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`
		_, err = db.ExecContext(r.Context(), query, sessionID, user.ID, mediaItemID, mediaItemType, playReq.StartTime, resolvedLibraryID, extraData)
		if err != nil {
			log.Printf("[handlePlayItem] Failed to insert session: %v", err)
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
			var audioFileJSONStr string
			var epTitle string
			err = db.QueryRowContext(r.Context(), `SELECT title, audioFile FROM podcastEpisodes WHERE id = ?`, mediaItemID).Scan(&epTitle, &audioFileJSONStr)
			if err == nil {
				displayTitle = epTitle

				var podcastID string
				_ = db.QueryRowContext(r.Context(), `SELECT podcastId FROM podcastEpisodes WHERE id = ?`, mediaItemID).Scan(&podcastID)
				if podcastID != "" {
					var podAuthor string
					_ = db.QueryRowContext(r.Context(), `SELECT author FROM podcasts WHERE id = ?`, podcastID).Scan(&podAuthor)
					displayAuthor = podAuthor
				}

				type AudioFileStruct struct {
					Duration float64 `json:"duration"`
					Codec    string  `json:"codec"`
					MimeType string  `json:"mimeType"`
					Metadata struct {
						Path string `json:"path"`
					} `json:"metadata"`
				}
				var audioFile AudioFileStruct
				if err := json.Unmarshal([]byte(audioFileJSONStr), &audioFile); err == nil {
					audioTracks = append(audioTracks, map[string]interface{}{
						"index":       0,
						"startOffset": 0.0,
						"duration":    audioFile.Duration,
						"title":       epTitle,
						"contentUrl":  fmt.Sprintf("/hls/%s/output.m3u8", sessionID),
						"mimeType":    audioFile.MimeType,
						"metadata": map[string]interface{}{
							"path": audioFile.Metadata.Path,
						},
					})
				}
			}
		} else {
			// Book
			var bTitle string
			err = db.QueryRowContext(r.Context(), `SELECT title FROM books WHERE id = ?`, mediaItemID).Scan(&bTitle)
			if err == nil {
				displayTitle = bTitle

				// Get book authors
				var authorNames []string
				rows, errAuthors := db.QueryContext(r.Context(), "SELECT name FROM authors WHERE id IN (SELECT authorId FROM bookAuthors WHERE bookId = ?)", mediaItemID)
				if errAuthors == nil {
					defer rows.Close()
					for rows.Next() {
						var name string
						if err := rows.Scan(&name); err == nil {
							authorNames = append(authorNames, name)
						}
					}
				}
				displayAuthor = strings.Join(authorNames, ", ")

				var audioFilesJSONStr string
				err = db.QueryRowContext(r.Context(), `SELECT audioFiles FROM books WHERE id = ?`, mediaItemID).Scan(&audioFilesJSONStr)
				if err == nil {
					type AudioFileJSON struct {
						Index    int     `json:"index"`
						Exclude  bool    `json:"exclude"`
						Duration float64 `json:"duration"`
						Codec    string  `json:"codec"`
						MimeType string  `json:"mimeType"`
						Metadata struct {
							Path     string `json:"path"`
							Filename string `json:"filename"`
							Size     int64  `json:"size"`
						} `json:"metadata"`
					}
					var audioFiles []AudioFileJSON
					if err := json.Unmarshal([]byte(audioFilesJSONStr), &audioFiles); err == nil {
						var currentOffset float64 = 0.0
						for _, af := range audioFiles {
							if !af.Exclude {
								audioTracks = append(audioTracks, map[string]interface{}{
									"index":       af.Index,
									"startOffset": currentOffset,
									"duration":    af.Duration,
									"title":       af.Metadata.Filename,
									"contentUrl":  fmt.Sprintf("/hls/%s/output.m3u8", sessionID),
									"mimeType":    af.MimeType,
									"metadata": map[string]interface{}{
										"path":     af.Metadata.Path,
										"filename": af.Metadata.Filename,
										"size":     af.Metadata.Size,
									},
								})
								currentOffset += af.Duration
							}
						}
					}
				}
			}
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
