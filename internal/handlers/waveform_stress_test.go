package handlers

import (
	"math"
	"testing"
)

// TestWaveform_FloatingPointEdgeCases verifies that GenerateWaveform is robust
// against NaN, Inf, and other extreme floating point durations and does not panic.
func TestWaveform_FloatingPointEdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		infos        []AudioFileInfo
		targetPoints int
	}{
		{
			name: "Single NaN duration",
			infos: []AudioFileInfo{
				{Path: "/nonexistent/nan.mp3", Duration: math.NaN()},
			},
			targetPoints: 200,
		},
		{
			name: "Mixed NaN and normal duration",
			infos: []AudioFileInfo{
				{Path: "/nonexistent/normal.mp3", Duration: 120.0},
				{Path: "/nonexistent/nan.mp3", Duration: math.NaN()},
			},
			targetPoints: 200,
		},
		{
			name: "Positive Infinity duration",
			infos: []AudioFileInfo{
				{Path: "/nonexistent/inf.mp3", Duration: math.Inf(1)},
			},
			targetPoints: 200,
		},
		{
			name: "Negative Infinity duration",
			infos: []AudioFileInfo{
				{Path: "/nonexistent/neginf.mp3", Duration: math.Inf(-1)},
			},
			targetPoints: 200,
		},
		{
			name: "Mixed Inf, NaN, and normal durations",
			infos: []AudioFileInfo{
				{Path: "/nonexistent/normal1.mp3", Duration: 50.0},
				{Path: "/nonexistent/nan.mp3", Duration: math.NaN()},
				{Path: "/nonexistent/inf.mp3", Duration: math.Inf(1)},
				{Path: "/nonexistent/neginf.mp3", Duration: math.Inf(-1)},
				{Path: "/nonexistent/normal2.mp3", Duration: 150.0},
			},
			targetPoints: 200,
		},
		{
			name: "Extremely large positive duration",
			infos: []AudioFileInfo{
				{Path: "/nonexistent/large.mp3", Duration: 1e300},
				{Path: "/nonexistent/normal.mp3", Duration: 10.0},
			},
			targetPoints: 200,
		},
		{
			name: "Extremely small positive duration",
			infos: []AudioFileInfo{
				{Path: "/nonexistent/small.mp3", Duration: 1e-300},
				{Path: "/nonexistent/normal.mp3", Duration: 10.0},
			},
			targetPoints: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("PANIC detected in GenerateWaveform for case %s: %v", tt.name, r)
				}
			}()

			peaks, err := GenerateWaveform(tt.infos, tt.targetPoints)
			if err != nil {
				// Errors are acceptable behavior for malformed inputs, but panics are not.
				t.Logf("Returned error (as expected or handled): %v", err)
			} else {
				if len(peaks) != tt.targetPoints {
					t.Errorf("Expected %d peaks, got %d", tt.targetPoints, len(peaks))
				}
			}
		})
	}
}
