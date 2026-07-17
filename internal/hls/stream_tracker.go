package hls

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"
)

// CheckSegmentNumberRequest determines if a requested segment falls outside the active transcode window.
func (s *Stream) CheckSegmentNumberRequest(segNum int) (float64, bool) {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	if s.isTranscodeComplete {
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

			if hasEnoughBuffer {
				s.stateMu.Lock()
				if !s.isClientInitialized {
					s.isClientInitialized = true
					s.stateMu.Unlock()
					emitWebsocketEvent(s.socketAuth, s.UserID, "stream_open", s.ToJSON())
				} else {
					s.stateMu.Unlock()
				}
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
