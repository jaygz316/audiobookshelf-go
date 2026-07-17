package hls

import (
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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

		streamID, fileName, ext, err := parseHLSRequestPath(r.URL.Path)
		if err != nil {
			log.Printf("[HLS Gateway] Bad Request: %v", err)
			http.Error(w, "Bad Request", http.StatusBadRequest)
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

		if !stream.waitForSegment(r.Context(), fullFilePath, streamID, fileName, ext, socketAuth) {
			if r.Context().Err() != nil {
				return
			}
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
