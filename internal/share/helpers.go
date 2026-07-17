package share

import (
	"fmt"
	"strconv"
	"time"
)

// Helper functions for formatting and parsing SQLite datetime strings

func timeToDBStr(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05.000 +00:00")
}

func parseTimeStr(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}

	layouts := []string{
		"2006-01-02 15:04:05.000 +00:00",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02 15:04:05.000000 +00:00",
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}

	// Fallback to millisecond unix timestamp parsing
	if val, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(0, val*int64(time.Millisecond)).UTC(), nil
	}

	return time.Time{}, fmt.Errorf("failed to parse datetime: %s", s)
}
