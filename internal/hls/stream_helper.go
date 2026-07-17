package hls

import (
	"fmt"
	"os"
	"strings"
)

func (s *Stream) needsAACForceLocked() bool {
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

func (s *Stream) needsAACForce() bool {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.needsAACForceLocked()
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
