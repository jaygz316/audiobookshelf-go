package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
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
	segmentsCreated     map[int]bool
	furthestSegCreated  int
	isClientInitialized bool
}

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

// LoadOrCreateStream retrieves a stream if cached, or initializes a new one from the database.
func (sm *StreamManager) LoadOrCreateStream(streamID string, metadataPath string) (*Stream, error) {
	sm.streamsMu.Lock()
	defer sm.streamsMu.Unlock()

	// Return cached stream if it exists
	if s, ok := sm.streams[streamID]; ok {
		return s, nil
	}

	if globalDB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var userID, mediaItemID, mediaItemType string
	var startTime float64
	var extraDataStr sql.NullString

	query := `SELECT userId, mediaItemId, mediaItemType, startTime, extraData FROM playbackSessions WHERE id = ?`
	err := globalDB.QueryRow(query, streamID).Scan(&userID, &mediaItemID, &mediaItemType, &startTime, &extraDataStr)
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
		err = globalDB.QueryRow(`SELECT audioFile FROM podcastEpisodes WHERE id = ?`, episodeID).Scan(&audioFileJSONStr)
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
		err = globalDB.QueryRow(`SELECT audioFiles FROM books WHERE id = ?`, mediaItemID).Scan(&audioFilesJSONStr)
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
		segmentsCreated:   make(map[int]bool),
		closeCancel:       closeCancel,
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

	emitWebsocketEvent(s.UserID, "stream_closed", s.ID)
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

	emitWebsocketEvent(s.UserID, "stream_error", map[string]interface{}{
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
				emitWebsocketEvent(s.UserID, "stream_ready", s.ID)
				return
			}

			s.scanCreatedSegments()

			total := s.TotalSegments()
			s.segmentsMu.RLock()
			createdCount := len(s.segmentsCreated)
			s.segmentsMu.RUnlock()

			percent := 0.0
			if total > 0 {
				percent = (float64(createdCount) / float64(total)) * 100
			}
			emitWebsocketEvent(s.UserID, "stream_progress", map[string]interface{}{
				"stream":      s.ID,
				"percent":     fmt.Sprintf("%.2f%%", percent),
				"numSegments": total,
			})

			s.segmentsMu.RLock()
			hasEnoughBuffer := len(s.segmentsCreated) > 6
			s.segmentsMu.RUnlock()

			if hasEnoughBuffer && !s.isClientInitialized {
				s.isClientInitialized = true
				emitWebsocketEvent(s.UserID, "stream_open", s.ToJSON())
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
				s.segmentsCreated[segNum] = true
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

func escapeSingleQuotes(path string) string {
	p := filepath.ToSlash(path)
	return strings.ReplaceAll(p, "'", "'\\''")
}

var segNumRegex = regexp.MustCompile(`output-(\d+)\.ts`)

func parseSegmentNumber(name string) int {
	matches := segNumRegex.FindStringSubmatch(name)
	if len(matches) < 2 {
		return -1
	}
	num, err := strconv.Atoi(matches[1])
	if err != nil {
		return -1
	}
	return num
}

func getPlaylistStr(segmentName string, duration float64, segmentLength float64, hlsSegmentType string) string {
	ext := "ts"
	if hlsSegmentType == "fmp4" {
		ext = "m4s"
	}
	lines := []string{
		"#EXTM3U",
		"#EXT-X-VERSION:3",
		"#EXT-X-ALLOW-CACHE:NO",
		"#EXT-X-TARGETDURATION:" + fmt.Sprintf("%.0f", segmentLength),
		"#EXT-X-MEDIA-SEQUENCE:0",
		"#EXT-X-PLAYLIST-TYPE:VOD",
	}
	if hlsSegmentType == "fmp4" {
		lines = append(lines, `#EXT-X-MAP:URI="init.mp4"`)
	}
	numSegments := int(duration / segmentLength)
	lastSegment := duration - float64(numSegments)*segmentLength
	for i := 0; i < numSegments; i++ {
		lines = append(lines, fmt.Sprintf("#EXTINF:%.0f,", segmentLength))
		lines = append(lines, fmt.Sprintf("%s-%d.%s", segmentName, i, ext))
	}
	if lastSegment > 0 {
		lines = append(lines, fmt.Sprintf("#EXTINF:%g,", lastSegment))
		lines = append(lines, fmt.Sprintf("%s-%d.%s", segmentName, numSegments, ext))
	}
	lines = append(lines, "#EXT-X-ENDLIST")
	return strings.Join(lines, "\n")
}

func emitWebsocketEvent(userID string, event string, payload interface{}) {
	if SocketAuth != nil {
		SocketAuth.ClientEmitter(userID, event, payload)
	} else {
		log.Printf("[HLS Stream Warning] SocketAuth not initialized, cannot emit: %s", event)
	}
}

// serveHLS returns an HTTP handler for intercepting HLS playlist and segment requests.
func serveHLS(metadataPath string, sm *StreamManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		streamID := parts[hlsIdx+1]
		fileName := parts[hlsIdx+2]

		ext := filepath.Ext(fileName)
		if ext != ".ts" && ext != ".m3u8" && ext != ".mp4" && ext != ".m4s" {
			http.Error(w, "Unsupported file format", http.StatusBadRequest)
			return
		}

		stream, err := sm.LoadOrCreateStream(streamID, metadataPath)
		if err != nil {
			log.Printf("[HLS Gateway] Error loading or creating stream %s: %v", streamID, err)
			http.Error(w, "Stream not found", http.StatusNotFound)
			return
		}

		fullFilePath := filepath.Join(stream.StreamPath, fileName)

		if _, err := os.Stat(fullFilePath); os.IsNotExist(err) {
			if ext == ".ts" {
				segNum := parseSegmentNumber(fileName)
				if segNum >= 0 {
					sTime, shouldReset := stream.CheckSegmentNumberRequest(segNum)
					if shouldReset {
						log.Printf("[HLS Gateway] Resetting stream %s at segment %d (time %.2fs)", streamID, segNum, sTime)
						go func() {
							_ = stream.Reset(sTime - (stream.SegmentLength * 5.0))
						}()
						emitWebsocketEvent(stream.UserID, "stream_reset", map[string]interface{}{
							"startTime": sTime,
							"streamId":  streamID,
						})
					}
				}
			}
			http.Error(w, "Segment not ready", http.StatusNotFound)
			return
		}

		w.Header().Set("Access-Control-Allow-Origin", "*")
		if ext == ".m3u8" {
			w.Header().Set("Content-Type", "application/x-mpegURL")
		} else if ext == ".ts" {
			w.Header().Set("Content-Type", "video/MP2T")
		}
		http.ServeFile(w, r, fullFilePath)
	}
}
