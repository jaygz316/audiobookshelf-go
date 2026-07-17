package handlers

import (
	"database/sql"
	"encoding/json"
	"time"

	idb "audiobookshelf/internal/db"
)

// currentProgressInfo stores existing media progress details read from the database.
type currentProgressInfo struct {
	exists                    bool
	duration                  float64
	currentTime               float64
	isFinished                int
	hideFromContinueListening int
	ebookLocation             sql.NullString
	finishedAt                sql.NullString
	extraData                 sql.NullString
	ebookProgress             sql.NullFloat64
}

// updatedProgressInfo stores the calculated results to be saved to the database.
type updatedProgressInfo struct {
	duration                  float64
	currentTime               float64
	isFinished                bool
	hideFromContinueListening bool
	ebookLocationNullable     interface{}
	ebookProgress             float64
	finishedAtNullable        interface{}
	extraBytes                []byte
	updatedAtStr              string
}

// calculateUpdatedProgress computes the new progress state based on the payload, the current state, and the current timestamp.
func calculateUpdatedProgress(payload *createUpdateMeProgressPayload, curr currentProgressInfo, libraryItemID string, now time.Time) (updatedProgressInfo, error) {
	nowStr := idb.TimeToDBStr(now)

	durationVal := curr.duration
	if payload.Duration != nil {
		durationVal = *payload.Duration
	}
	currentTimeVal := curr.currentTime
	if payload.CurrentTime != nil {
		currentTimeVal = *payload.CurrentTime
	}

	isFinishedVal := curr.isFinished != 0
	finishedAtVal := curr.finishedAt.String

	var extra map[string]interface{}
	if curr.exists && curr.extraData.Valid && curr.extraData.String != "" {
		json.Unmarshal([]byte(curr.extraData.String), &extra)
	}
	if extra == nil {
		extra = make(map[string]interface{})
	}
	extra["libraryItemId"] = libraryItemID

	if payload.IsFinished != nil {
		isFinishedVal = *payload.IsFinished
		if isFinishedVal && (curr.isFinished == 0) {
			if payload.FinishedAt != nil {
				finishedAtVal = idb.TimeToDBStr(time.UnixMilli(*payload.FinishedAt))
			} else {
				finishedAtVal = nowStr
			}
			extra["progress"] = 1.0
		} else if !isFinishedVal && (curr.isFinished != 0) {
			finishedAtVal = ""
			extra["progress"] = 0.0
			currentTimeVal = 0
		}
	} else if payload.Progress != nil {
		extra["progress"] = *payload.Progress
	} else if durationVal > 0 {
		extra["progress"] = currentTimeVal / durationVal
	}

	hideFromContinueListeningVal := curr.hideFromContinueListening != 0
	if payload.HideFromContinueListening != nil {
		hideFromContinueListeningVal = *payload.HideFromContinueListening
	} else if payload.CurrentTime != nil && currentTimeVal != curr.currentTime {
		// Reset hide if current time changed and hide was not explicitly specified
		hideFromContinueListeningVal = false
	}

	ebookLocationVal := curr.ebookLocation.String
	if payload.EbookLocation != nil {
		ebookLocationVal = *payload.EbookLocation
	}
	ebookProgressVal := curr.ebookProgress.Float64
	if payload.EbookProgress != nil {
		ebookProgressVal = *payload.EbookProgress
	}

	// Calculate progress pct and auto finish
	progPct := 0.0
	if durationVal > 0 {
		progPct = currentTimeVal / durationVal
	}

	shouldMarkAsFinished := false
	if durationVal > 0 {
		if payload.MarkAsFinishedPercentComplete != nil && *payload.MarkAsFinishedPercentComplete > 0 {
			shouldMarkAsFinished = (progPct * 100) > *payload.MarkAsFinishedPercentComplete
		} else {
			timeRemaining := durationVal - currentTimeVal
			timeRemLimit := 10.0
			if payload.MarkAsFinishedTimeRemaining != nil {
				timeRemLimit = *payload.MarkAsFinishedTimeRemaining
			}
			shouldMarkAsFinished = timeRemaining < timeRemLimit
		}
	}

	if !isFinishedVal && shouldMarkAsFinished {
		isFinishedVal = true
		if finishedAtVal == "" {
			finishedAtVal = nowStr
		}
		extra["progress"] = 1.0
	} else if isFinishedVal && (payload.CurrentTime != nil && currentTimeVal != curr.currentTime) && !shouldMarkAsFinished {
		isFinishedVal = false
		finishedAtVal = ""
	}

	extraBytes, err := json.Marshal(extra)
	if err != nil {
		return updatedProgressInfo{}, err
	}
	updatedAtStr := nowStr
	if payload.LastUpdate != nil {
		updatedAtStr = idb.TimeToDBStr(time.UnixMilli(*payload.LastUpdate))
	}

	var finishedAtNullable interface{} = nil
	if finishedAtVal != "" {
		finishedAtNullable = finishedAtVal
	}

	var ebookLocationNullable interface{} = nil
	if ebookLocationVal != "" {
		ebookLocationNullable = ebookLocationVal
	}

	return updatedProgressInfo{
		duration:                  durationVal,
		currentTime:               currentTimeVal,
		isFinished:                isFinishedVal,
		hideFromContinueListening: hideFromContinueListeningVal,
		ebookLocationNullable:     ebookLocationNullable,
		ebookProgress:             ebookProgressVal,
		finishedAtNullable:        finishedAtNullable,
		extraBytes:                extraBytes,
		updatedAtStr:              updatedAtStr,
	}, nil
}
