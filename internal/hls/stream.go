package hls

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	log "audiobookshelf/internal/logger"
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

// Start spawns the FFmpeg transcoding process.
func (s *Stream) Start() error {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	if s.ffmpegCancel != nil {
		s.ffmpegCancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.ffmpegCancel = cancel

	trackStartTime, err := s.writeConcatFile(s.Tracks)
	if err != nil {
		cancel()
		s.ffmpegCancel = nil
		return err
	}

	if s.StartTime > 0 {
		s.AdjustedStartTime = math.Max(s.StartTime-30.0, 0.0)
		s.SegmentStartNumber = int(math.Floor(s.AdjustedStartTime / s.SegmentLength))
	} else {
		s.AdjustedStartTime = 0.0
		s.SegmentStartNumber = 0
	}

	shiftedStartTime := s.AdjustedStartTime - trackStartTime

	args := []string{
		"-seek_timestamp", "1",
		"-safe", "0",
		"-f", "concat",
	}

	if s.AdjustedStartTime > 0 {
		args = append(args,
			"-ss", fmt.Sprintf("%.1fs", shiftedStartTime),
			"-noaccurate_seek",
		)
	}

	args = append(args, "-i", s.ConcatFilesPath)

	audioCodec := "copy"
	if s.needsAACForce() {
		audioCodec = "aac"
	}

	args = append(args,
		"-loglevel", "warning",
		"-map", "0:a",
		"-c:a", audioCodec,
		"-f", "hls",
		"-copyts",
		"-avoid_negative_ts", "make_non_negative",
		"-max_delay", "5000000",
		"-max_muxing_queue_size", "2048",
		"-hls_time", fmt.Sprintf("%.0f", s.SegmentLength),
		"-hls_segment_type", "mpegts",
		"-start_number", fmt.Sprintf("%d", s.SegmentStartNumber),
		"-hls_playlist_type", "vod",
		"-hls_list_size", "0",
		"-hls_allow_cache", "0",
		"-hls_segment_filename", filepath.Join(s.StreamPath, "output-%d.ts"),
		s.FinalPlaylistPath,
	)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	s.ffmpegCmd = cmd

	if err := cmd.Start(); err != nil {
		cancel()
		s.ffmpegCancel = nil
		return err
	}

	s.segmentsMu.Lock()
	s.furthestSegCreated = 0
	s.segmentsMu.Unlock()

	go func() {
		err := cmd.Wait()
		s.stateMu.Lock()
		defer s.stateMu.Unlock()

		s.isTranscodeComplete = true
		s.ffmpegCmd = nil
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.ExitStatus() == 1 {
					if audioCodec == "copy" {
						log.Printf("[HLS Stream] Transcode failed with copy codec, resetting to force AAC")
						s.isResettingToAAC = true
						s.stateMu.Unlock()
						s.Reset(s.StartTime)
						s.stateMu.Lock()
					}
				}
			}
		}
	}()

	return nil
}

// Reset terminates the current FFmpeg run and restarts it at the given time.
func (s *Stream) Reset(time float64) error {
	s.stateMu.Lock()
	if s.isResetting {
		s.stateMu.Unlock()
		return nil
	}
	s.isResetting = true
	s.stateMu.Unlock()

	s.KillFFmpeg()

	s.stateMu.Lock()
	s.isTranscodeComplete = false
	s.StartTime = math.Max(0, time)
	s.isResetting = false
	s.stateMu.Unlock()

	return s.Start()
}

// KillFFmpeg kills the running FFmpeg process group.
func (s *Stream) KillFFmpeg() {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	if s.ffmpegCancel != nil {
		s.ffmpegCancel()
		s.ffmpegCancel = nil
	}

	if s.ffmpegCmd != nil && s.ffmpegCmd.Process != nil {
		pgid, err := syscall.Getpgid(s.ffmpegCmd.Process.Pid)
		if err == nil {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		} else {
			_ = s.ffmpegCmd.Process.Kill()
		}
		s.ffmpegCmd = nil
	}
}

// Close terminates and cleans up the stream directory.
func (s *Stream) Close() {
	if s.closeCancel != nil {
		s.closeCancel()
	}
	s.KillFFmpeg()

	if s.StreamPath != "" {
		_ = os.RemoveAll(s.StreamPath)
		log.Printf("[HLS Stream] Closed and cleaned up stream path %s", s.StreamPath)
	}

	emitWebsocketEvent(s.socketAuth, s.UserID, "stream_closed", s.ID)
}

// CloseWithError terminates, cleans up, and emits an error event.
func (s *Stream) CloseWithError(errMsg string) {
	if s.closeCancel != nil {
		s.closeCancel()
	}
	s.KillFFmpeg()

	if s.StreamPath != "" {
		_ = os.RemoveAll(s.StreamPath)
	}

	emitWebsocketEvent(s.socketAuth, s.UserID, "stream_error", map[string]interface{}{
		"id":    s.ID,
		"error": errMsg,
	})
}

// CheckSegmentNumberRequest determines if a requested segment falls outside the active transcode window.
func (s *Stream) CheckSegmentNumberRequest(segNum int) (float64, bool) {
	s.stateMu.RLock()
	isComplete := s.isTranscodeComplete
	s.stateMu.RUnlock()

	if isComplete {
		return 0, false
	}

	segStartTime := float64(segNum) * s.SegmentLength

	if segNum < s.SegmentStartNumber {
		return segStartTime, true
	}

	s.segmentsMu.RLock()
	furthest := s.furthestSegCreated
	s.segmentsMu.RUnlock()

	if furthest > 0 {
		diff := segNum - furthest
		if diff > 10 {
			return segStartTime, true
		}
	}

	return 0, false
}

// RunProgressTracker scans segment files periodically and emits WebSocket events.
func (s *Stream) RunProgressTracker(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.stateMu.RLock()
			complete := s.isTranscodeComplete
			s.stateMu.RUnlock()

			if complete {
				emitWebsocketEvent(s.socketAuth, s.UserID, "stream_ready", s.ID)
				return
			}

			s.scanCreatedSegments()

			total := s.TotalSegments()
			s.segmentsMu.RLock()
			createdCount := len(s.SegmentsCreated)
			s.segmentsMu.RUnlock()

			percent := 0.0
			if total > 0 {
				percent = (float64(createdCount) / float64(total)) * 100
			}
			emitWebsocketEvent(s.socketAuth, s.UserID, "stream_progress", map[string]interface{}{
				"stream":      s.ID,
				"percent":     fmt.Sprintf("%.2f%%", percent),
				"numSegments": total,
			})

			s.segmentsMu.RLock()
			hasEnoughBuffer := len(s.SegmentsCreated) > 6
			s.segmentsMu.RUnlock()

			if hasEnoughBuffer && !s.isClientInitialized {
				s.isClientInitialized = true
				emitWebsocketEvent(s.socketAuth, s.UserID, "stream_open", s.ToJSON())
			}
		}
	}
}

// TotalSegments returns the total segment count for the audiobook.
func (s *Stream) TotalSegments() int {
	totalDuration := s.totalDuration()
	numSegs := int(math.Floor(totalDuration / s.SegmentLength))
	if totalDuration-float64(numSegs)*s.SegmentLength > 0 {
		numSegs++
	}
	return numSegs
}

// scanCreatedSegments checks directory files to identify newly created segments.
func (s *Stream) scanCreatedSegments() {
	files, err := os.ReadDir(s.StreamPath)
	if err != nil {
		return
	}

	s.segmentsMu.Lock()
	defer s.segmentsMu.Unlock()

	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		if filepath.Ext(name) == ".ts" {
			segNum := parseSegmentNumber(name)
			if segNum >= 0 {
				s.SegmentsCreated[segNum] = true
				if segNum > s.furthestSegCreated {
					s.furthestSegCreated = segNum
				}
			}
		}
	}
}

func (s *Stream) totalDuration() float64 {
	var total float64
	for _, t := range s.Tracks {
		total += t.Duration
	}
	return total
}

func (s *Stream) needsAACForce() bool {
	if s.isResettingToAAC {
		return true
	}
	if len(s.Tracks) == 0 {
		return false
	}
	codec := strings.ToLower(s.Tracks[0].Codec)
	mime := strings.ToLower(s.Tracks[0].MimeType)

	codecsToForce := []string{"alac", "ac3", "eac3", "opus", "flac"}
	mimesToForce := []string{
		"audio/flac", "audio/opus", "audio/x-ms-wma", "audio/x-aiff",
		"audio/webm", "audio/webma", "audio/awb", "audio/caf", "audio/ogg",
	}

	for _, c := range codecsToForce {
		if codec == c {
			return true
		}
	}
	for _, m := range mimesToForce {
		if strings.HasPrefix(mime, m) || mime == m {
			return true
		}
	}
	return false
}

// ToJSON serialization helper.
func (s *Stream) ToJSON() map[string]interface{} {
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

func (s *Stream) writeConcatFile(tracks []Track) (float64, error) {
	var trackToStartWithIndex int
	var firstTrackStartTime float64

	if s.AdjustedStartTime > 0 {
		var currTrackEnd float64
		found := false
		for _, t := range tracks {
			currTrackEnd += t.Duration
			if s.AdjustedStartTime < currTrackEnd {
				firstTrackStartTime = currTrackEnd - t.Duration
				trackToStartWithIndex = t.Index
				found = true
				break
			}
		}
		if !found {
			if len(tracks) > 0 {
				lastTrack := tracks[len(tracks)-1]
				trackToStartWithIndex = lastTrack.Index
				var sum float64
				for i := 0; i < len(tracks)-1; i++ {
					sum += tracks[i].Duration
				}
				firstTrackStartTime = sum
			}
		}
	}

	var lines []string
	for _, t := range tracks {
		if t.Index >= trackToStartWithIndex {
			escapedPath := escapeSingleQuotes(t.Path)
			line := fmt.Sprintf("file '%s'\nduration %f", escapedPath, t.Duration)
			lines = append(lines, line)
		}
	}
	inputstr := strings.Join(lines, "\n\n")

	if err := os.MkdirAll(s.StreamPath, 0755); err != nil {
		return 0, fmt.Errorf("failed to create stream directory: %w", err)
	}

	if err := os.WriteFile(s.ConcatFilesPath, []byte(inputstr), 0644); err != nil {
		return 0, fmt.Errorf("failed to write concat file: %w", err)
	}

	return firstTrackStartTime, nil
}
