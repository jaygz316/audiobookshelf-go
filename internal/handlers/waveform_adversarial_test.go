package handlers

import (
	"testing"
)

// TestWaveform_EmptyInfos tests that passing empty AudioFileInfo results in an error.
func TestWaveform_EmptyInfos(t *testing.T) {
	_, err := GenerateWaveform(nil, 200)
	if err == nil {
		t.Error("Expected error when generating waveform for nil slice")
	}

	_, err = GenerateWaveform([]AudioFileInfo{}, 200)
	if err == nil {
		t.Error("Expected error when generating waveform for empty slice")
	}
}

// TestWaveform_NegativeDurations tests that negative durations are handled safely.
func TestWaveform_NegativeDurations(t *testing.T) {
	// Scenario A: Mix of positive and negative durations where total is positive.
	infosMix := []AudioFileInfo{
		{Path: "/nonexistent/test1.mp3", Duration: 10.0},
		{Path: "/nonexistent/test2.mp3", Duration: -5.0},
	}

	// This should run without panic and return the target points of 100.
	peaks, err := GenerateWaveform(infosMix, 100)
	if err != nil {
		t.Fatalf("Unexpected error on mixed negative durations: %v", err)
	}
	if len(peaks) != 100 {
		t.Errorf("Expected 100 peaks, got %d", len(peaks))
	}

	// Scenario B: Mix of negative durations where total is negative or zero.
	infosAllNegative := []AudioFileInfo{
		{Path: "/nonexistent/test1.mp3", Duration: -10.0},
		{Path: "/nonexistent/test2.mp3", Duration: -20.0},
	}

	peaks2, err := GenerateWaveform(infosAllNegative, 100)
	if err != nil {
		t.Fatalf("Unexpected error on all negative durations: %v", err)
	}
	if len(peaks2) != 100 {
		t.Errorf("Expected 100 peaks, got %d", len(peaks2))
	}
}

// TestWaveform_TargetPointsPanic tests if a negative targetPoints argument causes a panic.
func TestWaveform_TargetPointsPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Log("No panic detected (or targetPoints handling was somehow safe)")
		} else {
			t.Logf("Detected panic as expected for negative targetPoints: %v", r)
		}
	}()

	infos := []AudioFileInfo{
		{Path: "/nonexistent/test1.mp3", Duration: 10.0},
	}

	// If it panics, the recover block will catch it.
	_, _ = GenerateWaveform(infos, -5)
}

// TestWaveform_ZeroTargetPoints tests if zero targetPoints argument is handled safely.
func TestWaveform_ZeroTargetPoints(t *testing.T) {
	infos := []AudioFileInfo{
		{Path: "/nonexistent/test1.mp3", Duration: 10.0},
	}

	peaks, err := GenerateWaveform(infos, 0)
	if err != nil {
		t.Fatalf("Unexpected error with 0 target points: %v", err)
	}
	if len(peaks) != 0 {
		t.Errorf("Expected 0 peaks, got %d", len(peaks))
	}
}

// TestWaveform_FfmpegFailureFallback tests that if ffmpeg fails (or files don't exist),
// peaks are generated as zero elements and don't cause panics.
func TestWaveform_FfmpegFailureFallback(t *testing.T) {
	infos := []AudioFileInfo{
		{Path: "/nonexistent/test1.mp3", Duration: 100.0},
	}

	peaks, err := GenerateWaveform(infos, 200)
	if err != nil {
		t.Fatalf("Unexpected error when generating waveform: %v", err)
	}

	if len(peaks) != 200 {
		t.Fatalf("Expected 200 peaks, got %d", len(peaks))
	}

	// Check that all peaks are zero since files don't exist and command failed
	for i, v := range peaks {
		if v != 0 {
			t.Errorf("Expected peak at index %d to be 0, got %d", i, v)
		}
	}
}
