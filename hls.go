package main

// hls.go — thin wrapper re-exporting HLS types and functions from internal/hls.

import (
	ihls "audiobookshelf/internal/hls"
	isocket "audiobookshelf/internal/socket"
	"database/sql"
	"net/http"
)

// Track is an alias for the internal HLS Track type.
type Track = ihls.Track

// Stream is an alias for the internal HLS Stream type.
type Stream = ihls.Stream

// StreamManager is an alias for the internal HLS StreamManager type.
type StreamManager = ihls.StreamManager

// NewStreamManager creates a new StreamManager.
func NewStreamManager() *StreamManager {
	return ihls.NewStreamManager()
}

// serveHLS returns an HTTP handler for HLS streaming.
func serveHLS(metadataPath string, sm *StreamManager) http.HandlerFunc {
	return ihls.ServeHLS(globalDB, metadataPath, sm, SocketAuth)
}

// handlePlayItem returns an HTTP handler for creating a playback session.
func handlePlayItem(db *sql.DB, sm *StreamManager) http.HandlerFunc {
	return ihls.HandlePlayItem(db, sm)
}

// emitWebsocketEvent emits a websocket event to a user.
func emitWebsocketEvent(userID string, event string, payload interface{}) {
	var sa *isocket.Authority
	if SocketAuth != nil {
		sa = SocketAuth
	}
	if sa != nil {
		sa.ClientEmitter(userID, event, payload)
	}
}

// getPlaylistStr builds an HLS playlist string (wrapper for testing).
func getPlaylistStr(segmentName string, duration float64, segmentLength float64, hlsSegmentType string) string {
	return ihls.GetPlaylistStr(segmentName, duration, segmentLength, hlsSegmentType)
}
