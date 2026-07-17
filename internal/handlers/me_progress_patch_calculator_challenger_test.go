package handlers

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"
)

func TestCalculateUpdatedProgress_Challenger(t *testing.T) {
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

	t.Run("Duration and CurrentTime Combinations", func(t *testing.T) {
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
			t.Fatalf("unexpected error: %v", err)
		}
		if res.duration != 0.0 || res.currentTime != 10.0 {
			t.Errorf("expected duration=0, currentTime=10; got duration=%f, currentTime=%f", res.duration, res.currentTime)
		}

		// Test currentTime exceeding duration
		payload = createUpdateMeProgressPayload{
			CurrentTime: f64Ptr(120.0),
		}
		curr = currentProgressInfo{
			exists:      true,
			duration:    100.0,
			currentTime: 10.0,
			isFinished:  0,
		}
		res, err = calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.isFinished {
			t.Errorf("expected to be marked finished when currentTime > duration")
		}
	})

	t.Run("Short duration auto-finish issue", func(t *testing.T) {
		payload := createUpdateMeProgressPayload{
			CurrentTime: f64Ptr(0.0),
		}
		curr := currentProgressInfo{
			exists:      true,
			duration:    5.0,
			currentTime: 0.0,
			isFinished:  0,
		}
		res, err := calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.isFinished {
			t.Errorf("Expected short duration media of 5s to be auto-finished at currentTime=0.0")
		}
	})

	t.Run("Percent complete boundary conditions", func(t *testing.T) {
		payload := createUpdateMeProgressPayload{
			CurrentTime:                   f64Ptr(94.9),
			MarkAsFinishedPercentComplete: f64Ptr(95.0),
		}
		curr := currentProgressInfo{
			exists:      true,
			duration:    100.0,
			currentTime: 50.0,
			isFinished:  0,
		}
		res, err := calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.isFinished {
			t.Errorf("expected not finished at 94.9%%")
		}

		payload = createUpdateMeProgressPayload{
			CurrentTime:                   f64Ptr(95.1),
			MarkAsFinishedPercentComplete: f64Ptr(95.0),
		}
		res, err = calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.isFinished {
			t.Errorf("expected finished at 95.1%%")
		}
	})

	t.Run("HideFromContinueListening logic", func(t *testing.T) {
		payload := createUpdateMeProgressPayload{
			CurrentTime: f64Ptr(15.0),
		}
		curr := currentProgressInfo{
			exists:                    true,
			duration:                  100.0,
			currentTime:               10.0,
			hideFromContinueListening: 1,
			isFinished:                0,
		}
		res, err := calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.hideFromContinueListening {
			t.Errorf("expected hideFromContinueListening to reset to false when currentTime updates")
		}

		payload = createUpdateMeProgressPayload{
			CurrentTime:               f64Ptr(15.0),
			HideFromContinueListening: boolPtr(true),
		}
		res, err = calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.hideFromContinueListening {
			t.Errorf("expected hideFromContinueListening to remain true when explicitly requested in payload")
		}
	})

	t.Run("Explicit IsFinished override bug", func(t *testing.T) {
		payload := createUpdateMeProgressPayload{
			IsFinished:  boolPtr(true),
			CurrentTime: f64Ptr(50.0),
		}
		curr := currentProgressInfo{
			exists:      true,
			duration:    100.0,
			currentTime: 10.0,
			isFinished:  0,
		}
		res, err := calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.isFinished {
			t.Errorf("BUG: Expected isFinished to be false due to CurrentTime override")
		}
	})

	t.Run("Extra data JSON validation", func(t *testing.T) {
		curr := currentProgressInfo{
			exists:    true,
			duration:  100.0,
			extraData: sql.NullString{String: `{"foo":"bar"}`, Valid: true},
		}
		payload := createUpdateMeProgressPayload{}
		res, err := calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var extra map[string]interface{}
		if err := json.Unmarshal(res.extraBytes, &extra); err != nil {
			t.Fatalf("failed to unmarshal extraBytes: %v", err)
		}
		if extra["foo"] != "bar" {
			t.Errorf("expected foo=bar in extra, got %v", extra["foo"])
		}
		if extra["libraryItemId"] != "lib-1" {
			t.Errorf("expected libraryItemId=lib-1, got %v", extra["libraryItemId"])
		}
	})

	t.Run("Update FinishedAt when already finished", func(t *testing.T) {
		payload := createUpdateMeProgressPayload{
			IsFinished: boolPtr(true),
			FinishedAt: int64Ptr(1600000005000), // new finishedAt
		}
		curr := currentProgressInfo{
			exists:      true,
			duration:    100.0,
			currentTime: 100.0,
			isFinished:  1,
			finishedAt:  sql.NullString{String: "2020-09-13 12:26:40", Valid: true}, // old finishedAt
		}
		res, err := calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expectedOldFinishedAt := "2020-09-13 12:26:40"
		if res.finishedAtNullable != expectedOldFinishedAt {
			t.Errorf("Expected finishedAt to remain the old value %s, got %s", expectedOldFinishedAt, res.finishedAtNullable)
		}
	})

	t.Run("Negative values handling", func(t *testing.T) {
		payload := createUpdateMeProgressPayload{
			CurrentTime: f64Ptr(-10.0),
			Duration:    f64Ptr(-100.0),
		}
		curr := currentProgressInfo{
			exists:      true,
			duration:    100.0,
			currentTime: 10.0,
			isFinished:  0,
		}
		res, err := calculateUpdatedProgress(&payload, curr, "lib-1", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.duration != -100.0 || res.currentTime != -10.0 {
			t.Errorf("expected negative duration/currentTime to be accepted; got duration=%f, currentTime=%f", res.duration, res.currentTime)
		}
	})
}
