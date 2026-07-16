package hls

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	isocket "audiobookshelf/internal/socket"
	"audiobookshelf/internal/utils"
)

// StreamManager manages the lifecycle of active streams.
type StreamManager struct {
	streams   map[string]*Stream
	streamsMu sync.RWMutex
}

// NewStreamManager creates a new StreamManager.
func NewStreamManager() *StreamManager {
	return &StreamManager{
		streams: make(map[string]*Stream),
	}
}

// GetStream retrieves an active stream by its ID.
func (sm *StreamManager) GetStream(id string) *Stream {
	sm.streamsMu.RLock()
	defer sm.streamsMu.RUnlock()
	return sm.streams[id]
}

// AddStream adds an active stream.
func (sm *StreamManager) AddStream(s *Stream) {
	sm.streamsMu.Lock()
	defer sm.streamsMu.Unlock()
	sm.streams[s.ID] = s
}

// RemoveStream cleans up and removes an active stream.
func (sm *StreamManager) RemoveStream(id string) {
	sm.streamsMu.Lock()
	defer sm.streamsMu.Unlock()
	if s, ok := sm.streams[id]; ok {
		s.Close()
		delete(sm.streams, id)
	}
}

// Close terminates and cleans up all active streams in the manager.
func (sm *StreamManager) Close() {
	sm.streamsMu.Lock()
	defer sm.streamsMu.Unlock()
	for id, s := range sm.streams {
		s.Close()
		delete(sm.streams, id)
	}
}

// LoadOrCreateStream retrieves a stream if cached, or initializes a new one from the database.
func (sm *StreamManager) LoadOrCreateStream(db *sql.DB, streamID string, metadataPath string, socketAuth *isocket.Authority) (*Stream, error) {
	sm.streamsMu.Lock()
	defer sm.streamsMu.Unlock()

	if strings.Contains(streamID, "..") || strings.Contains(streamID, "/") || strings.Contains(streamID, "\\") {
		return nil, fmt.Errorf("invalid stream ID")
	}

	// Return cached stream if it exists
	if s, ok := sm.streams[streamID]; ok {
		return s, nil
	}

	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var userID, mediaItemID, mediaItemType string
	var startTime float64
	var extraDataStr sql.NullString

	query := `SELECT userId, mediaItemId, mediaItemType, startTime, extraData FROM playbackSessions WHERE id = ?`
	err := db.QueryRow(query, streamID).Scan(&userID, &mediaItemID, &mediaItemType, &startTime, &extraDataStr)
	if err != nil {
		return nil, fmt.Errorf("playback session not found in db: %w", err)
	}

	var libraryItemID string
	if extraDataStr.Valid && extraDataStr.String != "" {
		var extraData struct {
			LibraryItemID string `json:"libraryItemId"`
		}
		if err := json.Unmarshal([]byte(extraDataStr.String), &extraData); err == nil {
			libraryItemID = extraData.LibraryItemID
		}
	}
	if libraryItemID == "" {
		libraryItemID = mediaItemID
	}

	var episodeID string
	var tracks []Track

	if mediaItemType == "podcastEpisode" {
		episodeID = mediaItemID
		var audioFileJSONStr string
		err = db.QueryRow(`SELECT audioFile FROM podcastEpisodes WHERE id = ?`, episodeID).Scan(&audioFileJSONStr)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch podcast episode: %w", err)
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
			if !utils.IsSafeFilePath(db, metadataPath, audioFile.Metadata.Path) {
				return nil, fmt.Errorf("forbidden: unsafe audio file path: %s", audioFile.Metadata.Path)
			}
			tracks = append(tracks, Track{
				Index:    0,
				Duration: audioFile.Duration,
				Path:     audioFile.Metadata.Path,
				Codec:    audioFile.Codec,
				MimeType: audioFile.MimeType,
			})
		} else {
			return nil, fmt.Errorf("failed to parse podcast episode audioFile json: %w", err)
		}
	} else {
		var audioFilesJSONStr string
		err = db.QueryRow(`SELECT audioFiles FROM books WHERE id = ?`, mediaItemID).Scan(&audioFilesJSONStr)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch book: %w", err)
		}

		type AudioFileJSON struct {
			Index    int     `json:"index"`
			Exclude  bool    `json:"exclude"`
			Duration float64 `json:"duration"`
			Codec    string  `json:"codec"`
			MimeType string  `json:"mimeType"`
			Metadata struct {
				Path string `json:"path"`
			} `json:"metadata"`
		}
		var audioFiles []AudioFileJSON
		if err := json.Unmarshal([]byte(audioFilesJSONStr), &audioFiles); err == nil {
			for _, af := range audioFiles {
				if !af.Exclude {
					if !utils.IsSafeFilePath(db, metadataPath, af.Metadata.Path) {
						return nil, fmt.Errorf("forbidden: unsafe audio file path: %s", af.Metadata.Path)
					}
					tracks = append(tracks, Track{
						Index:    af.Index,
						Duration: af.Duration,
						Path:     af.Metadata.Path,
						Codec:    af.Codec,
						MimeType: af.MimeType,
					})
				}
			}
		} else {
			return nil, fmt.Errorf("failed to parse book audioFiles json: %w", err)
		}
	}

	if len(tracks) == 0 {
		return nil, fmt.Errorf("no tracks found for media item %s", mediaItemID)
	}

	streamPath := filepath.Join(metadataPath, "streams", streamID)
	closeCtx, closeCancel := context.WithCancel(context.Background())

	s := &Stream{
		ID:                streamID,
		UserID:            userID,
		LibraryItemID:     libraryItemID,
		EpisodeID:         episodeID,
		StartTime:         startTime,
		SegmentLength:     6.0,
		StreamPath:        streamPath,
		ConcatFilesPath:   filepath.Join(streamPath, "files.txt"),
		PlaylistPath:      filepath.Join(streamPath, "output.m3u8"),
		FinalPlaylistPath: filepath.Join(streamPath, "final-output.m3u8"),
		Tracks:            tracks,
		SegmentsCreated:   make(map[int]bool),
		closeCancel:       closeCancel,
		socketAuth:        socketAuth,
	}

	_ = os.MkdirAll(streamPath, 0755)
	if _, err := os.Stat(s.PlaylistPath); os.IsNotExist(err) {
		playlistStr := getPlaylistStr("output", s.totalDuration(), s.SegmentLength, "mpegts")
		_ = os.WriteFile(s.PlaylistPath, []byte(playlistStr), 0644)
	}

	if err := s.Start(); err != nil {
		closeCancel()
		return nil, fmt.Errorf("failed to start transcode: %w", err)
	}

	go s.RunProgressTracker(closeCtx)

	sm.streams[streamID] = s
	return s, nil
}
