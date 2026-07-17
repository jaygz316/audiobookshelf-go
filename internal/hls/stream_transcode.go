package hls

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"syscall"

	log "audiobookshelf/internal/logger"
)

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
	if s.needsAACForceLocked() {
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
		s.isTranscodeComplete = true
		s.ffmpegCmd = nil
		var shouldReset bool
		var startTime float64
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.ExitStatus() == 1 {
					if audioCodec == "copy" {
						log.Printf("[HLS Stream] Transcode failed with copy codec, resetting to force AAC")
						s.isResettingToAAC = true
						shouldReset = true
						startTime = s.StartTime
					}
				}
			}
		}
		s.stateMu.Unlock()

		if shouldReset {
			s.Reset(startTime)
		}
	}()

	return nil
}
