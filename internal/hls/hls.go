// Package hls provides HLS (HTTP Live Streaming) transcoding functionality.
package hls

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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

	"github.com/google/uuid"

	"audiobookshelf/internal/core"
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
func (sm *StreamManager) LoadOrCreateStream(db *sql.DB, streamID string, metadataPath string, socketAuth *isocket.Authority) (*Stream, error) {
	sm.streamsMu.Lock()
	defer sm.streamsMu.Unlock()

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

// GetPlaylistStr builds an HLS playlist string (exported for testing).
func GetPlaylistStr(segmentName string, duration float64, segmentLength float64, hlsSegmentType string) string {
	return getPlaylistStr(segmentName, duration, segmentLength, hlsSegmentType)
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

func emitWebsocketEvent(socketAuth *isocket.Authority, userID string, event string, payload interface{}) {
	if socketAuth != nil {
		socketAuth.ClientEmitter(userID, event, payload)
	} else {
		log.Printf("[HLS Stream Warning] SocketAuth not initialized, cannot emit: %s", event)
	}
}

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

		if itemID == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": "Invalid Item ID"}`))
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
