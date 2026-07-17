package hls

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	isocket "audiobookshelf/internal/socket"
	"audiobookshelf/internal/utils"
)

type audioFileJSON struct {
	Index    int     `json:"index"`
	Exclude  bool    `json:"exclude"`
	Duration float64 `json:"duration"`
	Codec    string  `json:"codec"`
	MimeType string  `json:"mimeType"`
	Metadata struct {
		Path string `json:"path"`
	} `json:"metadata"`
}

// LoadOrCreateStream retrieves a stream session if it already exists, or queries the database
// to construct, initialize, and start a new transcoding session under singleflight protection.
func (sm *StreamManager) LoadOrCreateStream(db *sql.DB, streamID string, metadataPath string, socketAuth *isocket.Authority) (*Stream, error) {
	if strings.Contains(streamID, "..") || strings.Contains(streamID, "/") || strings.Contains(streamID, "\\") {
		return nil, fmt.Errorf("invalid stream ID")
	}

	sm.streamsMu.RLock()
	if s, ok := sm.streams[streamID]; ok {
		sm.streamsMu.RUnlock()
		return s, nil
	}
	sm.streamsMu.RUnlock()

	res, err, _ := sm.sf.Do(streamID, func() (interface{}, error) {
		sm.streamsMu.RLock()
		if s, ok := sm.streams[streamID]; ok {
			sm.streamsMu.RUnlock()
			return s, nil
		}
		sm.streamsMu.RUnlock()

		if db == nil {
			return nil, fmt.Errorf("database not initialized")
		}

		var userID, mediaItemID, mediaItemType string
		var startTime float64
		var extraDataStr sql.NullString

		query := `SELECT userId, mediaItemId, mediaItemType, startTime, extraData FROM playbackSessions WHERE id = ?`
		if err := db.QueryRow(query, streamID).Scan(&userID, &mediaItemID, &mediaItemType, &startTime, &extraDataStr); err != nil {
			return nil, fmt.Errorf("playback session not found in db: %w", err)
		}

		libraryItemID := mediaItemID
		if extraDataStr.Valid && extraDataStr.String != "" {
			var extra struct {
				LibraryItemID string `json:"libraryItemId"`
			}
			if err := json.Unmarshal([]byte(extraDataStr.String), &extra); err == nil && extra.LibraryItemID != "" {
				libraryItemID = extra.LibraryItemID
			}
		}

		var episodeID string
		var tracks []Track

		if mediaItemType == "podcastEpisode" {
			episodeID = mediaItemID
			var audioFileJSONStr string
			if err := db.QueryRow(`SELECT audioFile FROM podcastEpisodes WHERE id = ?`, episodeID).Scan(&audioFileJSONStr); err != nil {
				return nil, fmt.Errorf("failed to fetch podcast episode: %w", err)
			}
			var af audioFileJSON
			if err := json.Unmarshal([]byte(audioFileJSONStr), &af); err != nil {
				return nil, fmt.Errorf("failed to parse podcast episode audioFile json: %w", err)
			}
			if !utils.IsSafeFilePath(db, metadataPath, af.Metadata.Path) {
				return nil, fmt.Errorf("forbidden: unsafe audio file path: %s", af.Metadata.Path)
			}
			tracks = append(tracks, Track{
				Index: 0, Duration: af.Duration, Path: af.Metadata.Path, Codec: af.Codec, MimeType: af.MimeType,
			})
		} else {
			var audioFilesJSONStr string
			if err := db.QueryRow(`SELECT audioFiles FROM books WHERE id = ?`, mediaItemID).Scan(&audioFilesJSONStr); err != nil {
				return nil, fmt.Errorf("failed to fetch book: %w", err)
			}
			var audioFiles []audioFileJSON
			if err := json.Unmarshal([]byte(audioFilesJSONStr), &audioFiles); err != nil {
				return nil, fmt.Errorf("failed to parse book audioFiles json: %w", err)
			}
			for _, af := range audioFiles {
				if !af.Exclude {
					if !utils.IsSafeFilePath(db, metadataPath, af.Metadata.Path) {
						return nil, fmt.Errorf("forbidden: unsafe audio file path: %s", af.Metadata.Path)
					}
					tracks = append(tracks, Track{
						Index: af.Index, Duration: af.Duration, Path: af.Metadata.Path, Codec: af.Codec, MimeType: af.MimeType,
					})
				}
			}
		}

		if len(tracks) == 0 {
			return nil, fmt.Errorf("no tracks found for media item %s", mediaItemID)
		}

		streamPath := filepath.Join(metadataPath, "streams", streamID)
		closeCtx, closeCancel := context.WithCancel(context.Background())

		s := &Stream{
			ID: streamID, UserID: userID, LibraryItemID: libraryItemID, EpisodeID: episodeID,
			StartTime: startTime, SegmentLength: 6.0, StreamPath: streamPath,
			ConcatFilesPath: filepath.Join(streamPath, "files.txt"), PlaylistPath: filepath.Join(streamPath, "output.m3u8"),
			FinalPlaylistPath: filepath.Join(streamPath, "final-output.m3u8"), Tracks: tracks,
			SegmentsCreated: make(map[int]bool), closeCancel: closeCancel, socketAuth: socketAuth,
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

		sm.streamsMu.Lock()
		sm.streams[streamID] = s
		sm.streamsMu.Unlock()

		return s, nil
	})

	if err != nil {
		return nil, err
	}
	return res.(*Stream), nil
}
