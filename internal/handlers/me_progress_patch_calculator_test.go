package handlers

import (
	"database/sql"
	"testing"
	"time"
)

func TestCalculateUpdatedProgress(t *testing.T) {
	now := time.UnixMilli(1600000000000)

	// Helper to create float64 pointer
	f64Ptr := func(v float64) *float64 {
		return &v
	}
	// Helper to create bool pointer
	boolPtr := func(v bool) *bool {
		return &v
	}
	// Helper to create int64 pointer
	int64Ptr := func(v int64) *int64 {
		return &v
	}
	// Helper to create string pointer
	strPtr := func(v string) *string {
		return &v
	}

	tests := []struct {
		name          string
		payload       createUpdateMeProgressPayload
		curr          currentProgressInfo
		libraryItemID string
		wantFinished  bool
		wantCurrent   float64
		wantDuration  float64
		wantErr       bool
	}{
		{
			name: "Basic current time update",
			payload: createUpdateMeProgressPayload{
				CurrentTime: f64Ptr(50.0),
			},
			curr: currentProgressInfo{
				exists:      true,
				duration:    100.0,
				currentTime: 10.0,
				isFinished:  0,
			},
			libraryItemID: "lib-1",
			wantFinished:  false,
			wantCurrent:   50.0,
			wantDuration:  100.0,
		},
		{
			name: "Mark as finished explicitly",
			payload: createUpdateMeProgressPayload{
				IsFinished: boolPtr(true),
			},
			curr: currentProgressInfo{
				exists:      true,
				duration:    100.0,
				currentTime: 50.0,
				isFinished:  0,
			},
			libraryItemID: "lib-1",
			wantFinished:  true,
			wantCurrent:   50.0,
			wantDuration:  100.0,
		},
		{
			name: "Unmark as finished",
			payload: createUpdateMeProgressPayload{
				IsFinished: boolPtr(false),
			},
			curr: currentProgressInfo{
				exists:      true,
				duration:    100.0,
				currentTime: 100.0,
				isFinished:  1,
			},
			libraryItemID: "lib-1",
			wantFinished:  false,
			wantCurrent:   0.0,
			wantDuration:  100.0,
		},
		{
			name: "Auto-finish based on time remaining limit",
			payload: createUpdateMeProgressPayload{
				CurrentTime:                 f64Ptr(95.0),
				MarkAsFinishedTimeRemaining: f64Ptr(10.0),
			},
			curr: currentProgressInfo{
				exists:      true,
				duration:    100.0,
				currentTime: 50.0,
				isFinished:  0,
			},
			libraryItemID: "lib-1",
			wantFinished:  true,
			wantCurrent:   95.0,
			wantDuration:  100.0,
		},
		{
			name: "Auto-finish based on percent complete",
			payload: createUpdateMeProgressPayload{
				CurrentTime:                   f64Ptr(91.0),
				MarkAsFinishedPercentComplete: f64Ptr(90.0),
			},
			curr: currentProgressInfo{
				exists:      true,
				duration:    100.0,
				currentTime: 50.0,
				isFinished:  0,
			},
			libraryItemID: "lib-1",
			wantFinished:  true,
			wantCurrent:   91.0,
			wantDuration:  100.0,
		},
		{
			name: "Ebook location and progress update",
			payload: createUpdateMeProgressPayload{
				EbookLocation: strPtr("chapter-2"),
				EbookProgress: f64Ptr(0.45),
			},
			curr: currentProgressInfo{
				exists:        true,
				ebookLocation: sql.NullString{String: "chapter-1", Valid: true},
				ebookProgress: sql.NullFloat64{Float64: 0.1, Valid: true},
			},
			libraryItemID: "lib-1",
			wantFinished:  false,
			wantCurrent:   0.0,
			wantDuration:  0.0,
		},
		{
			name: "Explicit finished at timestamp",
			payload: createUpdateMeProgressPayload{
				IsFinished: boolPtr(true),
				FinishedAt: int64Ptr(1600000005000),
			},
			curr: currentProgressInfo{
				exists:     true,
				duration:   200.0,
				isFinished: 0,
			},
			libraryItemID: "lib-1",
			wantFinished:  true,
			wantDuration:  200.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := calculateUpdatedProgress(&tt.payload, tt.curr, tt.libraryItemID, now)
			if (err != nil) != tt.wantErr {
				t.Fatalf("calculateUpdatedProgress() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if res.isFinished != tt.wantFinished {
				t.Errorf("expected isFinished = %v, got %v", tt.wantFinished, res.isFinished)
			}
			if res.currentTime != tt.wantCurrent {
				t.Errorf("expected currentTime = %v, got %v", tt.wantCurrent, res.currentTime)
			}
			if res.duration != tt.wantDuration {
				t.Errorf("expected duration = %v, got %v", tt.wantDuration, res.duration)
			}
		})
	}
}

func TestCalculateUpdatedProgressAdversarial(t *testing.T) {
	now := time.UnixMilli(1600000000000) // 2020-09-13 12:26:40 UTC
	nowStr := "2020-09-13 12:26:40.000 +00:00"

	f64Ptr := func(v float64) *float64 { return &v }
	boolPtr := func(v bool) *bool { return &v }
	int64Ptr := func(v int64) *int64 { return &v }

	t.Run("Zero and Negative Duration and CurrentTime", func(t *testing.T) {
		// Duration = 0, CurrentTime = 10
		payload := createUpdateMeProgressPayload{
			CurrentTime: f64Ptr(10.0),
		}
		curr := currentProgressInfo{
			exists:      true,
			duration:    0.0,
			currentTime: 0.0,
			isFinished:  0,
		}
		res, err := calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if res.duration != 0.0 {
			t.Errorf("Expected duration 0, got %f", res.duration)
		}
		if res.currentTime != 10.0 {
			t.Errorf("Expected currentTime 10, got %f", res.currentTime)
		}
		if res.isFinished {
			t.Errorf("Expected isFinished to be false")
		}

		// Duration = -50, CurrentTime = 10
		curr.duration = -50.0
		res, err = calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if res.duration != -50.0 {
			t.Errorf("Expected duration -50, got %f", res.duration)
		}
		if res.currentTime != 10.0 {
			t.Errorf("Expected currentTime 10, got %f", res.currentTime)
		}

		// Duration = 100, CurrentTime = -10
		payload.CurrentTime = f64Ptr(-10.0)
		curr.duration = 100.0
		res, err = calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if res.currentTime != -10.0 {
			t.Errorf("Expected currentTime -10, got %f", res.currentTime)
		}
	})

	t.Run("Auto-finish percent complete boundary conditions", func(t *testing.T) {
		// MarkAsFinishedPercentComplete = 100
		// Case A: progress is 99.9%
		payload := createUpdateMeProgressPayload{
			CurrentTime:                   f64Ptr(99.9),
			MarkAsFinishedPercentComplete: f64Ptr(100.0),
		}
		curr := currentProgressInfo{
			exists:      true,
			duration:    100.0,
			currentTime: 50.0,
			isFinished:  0,
		}
		res, err := calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if res.isFinished {
			t.Errorf("99.9%% should not trigger auto-finish when percent threshold is 100")
		}

		// Case B: progress is exactly 100.0%
		payload.CurrentTime = f64Ptr(100.0)
		res, err = calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if res.isFinished {
			t.Errorf("Exactly 100.0%% should NOT trigger auto-finish because logic uses strict greater-than (progPct * 100 > threshold)")
		}

		// Case C: progress is 100.1%
		payload.CurrentTime = f64Ptr(100.1)
		res, err = calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !res.isFinished {
			t.Errorf("100.1%% should trigger auto-finish when percent threshold is 100")
		}

		// Case D: MarkAsFinishedPercentComplete = 0 (fallback to time remaining)
		payload.CurrentTime = f64Ptr(95.0) // remaining = 5s
		payload.MarkAsFinishedPercentComplete = f64Ptr(0.0)
		payload.MarkAsFinishedTimeRemaining = f64Ptr(10.0)
		res, err = calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !res.isFinished {
			t.Errorf("Percent threshold = 0 should fall back to time remaining check and trigger finish (95/100, remaining 5 < 10)")
		}
	})

	t.Run("Auto-finish time remaining boundary conditions", func(t *testing.T) {
		// MarkAsFinishedTimeRemaining = 10
		// Case A: timeRemaining = 10.1s
		payload := createUpdateMeProgressPayload{
			CurrentTime:                 f64Ptr(89.9),
			MarkAsFinishedTimeRemaining: f64Ptr(10.0),
		}
		curr := currentProgressInfo{
			exists:      true,
			duration:    100.0,
			currentTime: 50.0,
			isFinished:  0,
		}
		res, err := calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if res.isFinished {
			t.Errorf("Time remaining 10.1s should not trigger finish when limit is 10s")
		}

		// Case B: timeRemaining = 10.0s
		payload.CurrentTime = f64Ptr(90.0)
		res, err = calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if res.isFinished {
			t.Errorf("Time remaining exactly 10.0s should NOT trigger finish because logic uses strict less-than (timeRemaining < limit)")
		}

		// Case C: timeRemaining = 9.9s
		payload.CurrentTime = f64Ptr(90.1)
		res, err = calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !res.isFinished {
			t.Errorf("Time remaining 9.9s should trigger finish when limit is 10s")
		}
	})

	t.Run("HideFromContinueListening reset and override", func(t *testing.T) {
		// Case A: hide = true, no time change -> remains true
		payload := createUpdateMeProgressPayload{}
		curr := currentProgressInfo{
			exists:                    true,
			hideFromContinueListening: 1,
			currentTime:               50.0,
		}
		res, err := calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !res.hideFromContinueListening {
			t.Errorf("Expected hide to remain true when time doesn't change and hide not specified")
		}

		// Case B: hide = true, time changes -> reset to false
		payload.CurrentTime = f64Ptr(60.0)
		res, err = calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if res.hideFromContinueListening {
			t.Errorf("Expected hide to reset to false when time changes and hide not specified")
		}

		// Case C: hide = true, time changes, but hide explicitly specified in payload -> override wins
		payload.HideFromContinueListening = boolPtr(true)
		res, err = calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !res.hideFromContinueListening {
			t.Errorf("Expected hide to stay true when explicitly requested even if time changes")
		}
	})

	t.Run("finishedAt behavior and override", func(t *testing.T) {
		// Case A: explicit finish with custom finishedAt
		payload := createUpdateMeProgressPayload{
			IsFinished: boolPtr(true),
			FinishedAt: int64Ptr(1600000005000), // 2020-09-13 12:26:45 UTC
		}
		curr := currentProgressInfo{
			exists:     true,
			isFinished: 0,
		}
		res, err := calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !res.isFinished {
			t.Errorf("Expected isFinished = true")
		}
		expectedFinishedAt := "2020-09-13 12:26:45.000 +00:00"
		if res.finishedAtNullable != expectedFinishedAt {
			t.Errorf("Expected finishedAt = %q, got %v", expectedFinishedAt, res.finishedAtNullable)
		}

		// Case B: explicit finish without finishedAt -> defaults to now
		payload.FinishedAt = nil
		res, err = calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if res.finishedAtNullable != nowStr {
			t.Errorf("Expected finishedAt = %q, got %v", nowStr, res.finishedAtNullable)
		}
	})

	t.Run("Explicit IsFinished=true but CurrentTime is outside auto-finish range", func(t *testing.T) {
		// If client sends IsFinished=true AND CurrentTime=50 (where duration=100)
		payload := createUpdateMeProgressPayload{
			IsFinished:                  boolPtr(true),
			CurrentTime:                 f64Ptr(50.0),
			MarkAsFinishedTimeRemaining: f64Ptr(10.0),
		}
		curr := currentProgressInfo{
			exists:      true,
			duration:    100.0,
			currentTime: 10.0,
			isFinished:  0,
		}
		res, err := calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if res.isFinished {
			t.Errorf("Expected isFinished to be overridden to false because CurrentTime is outside auto-finish range")
		}
	})
}
