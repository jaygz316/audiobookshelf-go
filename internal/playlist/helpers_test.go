package playlist

import (
	"testing"
	"time"
)

func TestTimeHelpers(t *testing.T) {
	// Test msToTimeStr and parseMsFromDBStr cycle
	nowMs := time.Now().UnixNano() / int64(time.Millisecond)
	timeStr := msToTimeStr(nowMs)
	parsedMs := parseMsFromDBStr(timeStr)

	// Since millisecond resolution is kept, nowMs and parsedMs should match.
	if nowMs != parsedMs {
		t.Errorf("expected original ms %d to match parsed ms %d (timeStr: %s)", nowMs, parsedMs, timeStr)
	}

	// Test invalid time strings return 0 without panicking
	if parsed := parseMsFromDBStr("invalid-time"); parsed != 0 {
		t.Errorf("expected 0 for invalid time, got %d", parsed)
	}
}
