package handlers

import (
	"testing"
)

func TestGenerateWaveform_Logic(t *testing.T) {
	// 1. Empty audio files info
	_, err := GenerateWaveform(nil, 200)
	if err == nil {
		t.Errorf("Expected error for empty audio files info")
	}

	// 2. Normal downsampling length check with fallback (non-existent files)
	infos := []AudioFileInfo{
		{Path: "/nonexistent/1.mp3", Duration: 10.0},
		{Path: "/nonexistent/2.mp3", Duration: 20.0},
	}
	peaks, err := GenerateWaveform(infos, 150)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(peaks) != 150 {
		t.Errorf("Expected 150 peaks, got %d", len(peaks))
	}

	// 3. Zero duration fallback logic
	zeroInfos := []AudioFileInfo{
		{Path: "/nonexistent/1.mp3", Duration: 0.0},
		{Path: "/nonexistent/2.mp3", Duration: 0.0},
	}
	peaksZero, err := GenerateWaveform(zeroInfos, 100)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(peaksZero) != 100 {
		t.Errorf("Expected 100 peaks, got %d", len(peaksZero))
	}
}
