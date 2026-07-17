package hls

import (
	"math"
	"os"
	"syscall"

	log "audiobookshelf/internal/logger"
)

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
	s.isClientInitialized = false
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
