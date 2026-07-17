package hls

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func TestStreamConcurrencyStress(t *testing.T) {
	// Construct a stream with mock data
	s := &Stream{
		ID:            "stress-test-stream",
		UserID:        "user-1",
		SegmentLength: 6.0,
		Tracks: []Track{
			{Index: 0, Duration: 120.0, Codec: "mp3", MimeType: "audio/mpeg"},
			{Index: 1, Duration: 180.0, Codec: "aac", MimeType: "audio/mp4"},
		},
		SegmentsCreated:    make(map[int]bool),
		SegmentStartNumber: 0,
	}

	// We will run multiple goroutines calling various methods concurrently.
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Goroutine 1: periodically updates SegmentsCreated (simulating progress tracker / scanCreatedSegments)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				s.segmentsMu.Lock()
				s.SegmentsCreated[i] = true
				if i > s.furthestSegCreated {
					s.furthestSegCreated = i
				}
				s.segmentsMu.Unlock()
				time.Sleep(1 * time.Millisecond)
			}
		}
	}()

	// Goroutine 2: runs CheckSegmentNumberRequest concurrently
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				segNum := rand.Intn(100)
				_, _ = s.CheckSegmentNumberRequest(segNum)
				time.Sleep(1 * time.Millisecond)
			}
		}()
	}

	// Goroutine 3: reads stats concurrently (TotalSegments, ToJSON)
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = s.TotalSegments()
				_ = s.ToJSON()
				_ = s.needsAACForce()
				time.Sleep(1 * time.Millisecond)
			}
		}()
	}

	// Goroutine 4: does some mocks of stateMu locking
	for g := 0; g < 5; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				s.stateMu.Lock()
				s.isTranscodeComplete = !s.isTranscodeComplete
				s.stateMu.Unlock()
				time.Sleep(2 * time.Millisecond)
			}
		}()
	}

	wg.Wait()
}

func BenchmarkStreamOperations(b *testing.B) {
	s := &Stream{
		ID:            "benchmark-stream",
		UserID:        "user-1",
		SegmentLength: 6.0,
		Tracks: []Track{
			{Index: 0, Duration: 120.0, Codec: "mp3", MimeType: "audio/mpeg"},
			{Index: 1, Duration: 180.0, Codec: "aac", MimeType: "audio/mp4"},
		},
		SegmentsCreated:    make(map[int]bool),
		SegmentStartNumber: 0,
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = s.TotalSegments()
			_ = s.ToJSON()
			_, _ = s.CheckSegmentNumberRequest(rand.Intn(50))
		}
	})
}
