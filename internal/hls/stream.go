package hls

import (
	"context"
	"fmt"
	"os/exec"
	"sync"

	isocket "audiobookshelf/internal/socket"
)

// Track represents an audio track inside the audiobook.
type Track struct {
	Index    int
	Duration float64
	Path     string
	Codec    string
	MimeType string
}

// Stream represents an active HLS transcoding session.
type Stream struct {
	ID                 string
	UserID             string
	LibraryItemID      string
	EpisodeID          string
	StartTime          float64 // Original client requested start time (sec)
	AdjustedStartTime  float64 // Buffer-shifted start time (sec)
	SegmentStartNumber int     // Index of first segment written by this transcode run
	SegmentLength      float64 // Typically 6 seconds

	StreamPath        string // Base directory containing HLS output files
	ConcatFilesPath   string // Path to files.txt concat input
	PlaylistPath      string // Path to output.m3u8 (pre-generated)
	FinalPlaylistPath string // Path to final-output.m3u8 (written by ffmpeg)

	Tracks           []Track
	isResettingToAAC bool

	// Process Control
	ffmpegCmd           *exec.Cmd
	ffmpegCancel        context.CancelFunc
	stateMu             sync.RWMutex
	isResetting         bool
	isTranscodeComplete bool
	closeCancel         context.CancelFunc

	// Segment Tracking
	segmentsMu          sync.RWMutex
	SegmentsCreated     map[int]bool
	furthestSegCreated  int
	isClientInitialized bool

	// Socket emitter (may be nil)
	socketAuth *isocket.Authority
}

// ToJSON serialization helper.
func (s *Stream) ToJSON() map[string]interface{} {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return map[string]interface{}{
		"id":                  s.ID,
		"userId":              s.UserID,
		"segmentLength":       s.SegmentLength,
		"playlistPath":        s.PlaylistPath,
		"clientPlaylistUri":   fmt.Sprintf("/hls/%s/output.m3u8", s.ID),
		"startTime":           s.StartTime,
		"segmentStartNumber":  s.SegmentStartNumber,
		"isTranscodeComplete": s.isTranscodeComplete,
	}
}

func (s *Stream) totalDuration() float64 {
	var total float64
	for _, t := range s.Tracks {
		total += t.Duration
	}
	return total
}
