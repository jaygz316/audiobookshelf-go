package hls

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestAdversarial_Concurrency(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "hls-adv-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	s := &Stream{
		ID:                 "test-stream",
		UserID:             "user-1",
		SegmentLength:      6.0,
		SegmentStartNumber: 0,
		StreamPath:         tempDir,
		ConcatFilesPath:    filepath.Join(tempDir, "files.txt"),
		SegmentsCreated:    make(map[int]bool),
		Tracks: []Track{
			{Index: 0, Duration: 60.0, Path: "dummy.mp3"},
		},
	}

	var wg sync.WaitGroup
	done := make(chan struct{})

	// Goroutine 1: Continually calling Reset
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				// Reset will call Start(), which tries to start ffmpeg.
				// Since "ffmpeg" may fail (or succeed/block), we want to test the data races on the fields.
				// If ffmpeg starts, we want to kill it, but we can also just run Reset.
				// Let's call Reset.
				_ = s.Reset(30.0)
				time.Sleep(1 * time.Millisecond)
			}
		}
	}()

	// Goroutine 2: Continually calling CheckSegmentNumberRequest
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				_, _ = s.CheckSegmentNumberRequest(10)
			}
		}
	}()

	// Goroutine 3: Continually calling ToJSON
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				_ = s.ToJSON()
			}
		}
	}()

	// Run for a short duration
	time.Sleep(100 * time.Millisecond)
	close(done)
	wg.Wait()

	// Clean up any remaining processes
	s.KillFFmpeg()
}
