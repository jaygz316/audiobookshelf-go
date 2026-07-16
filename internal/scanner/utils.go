package scanner

import (
	"database/sql"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

func uuidStr() string {
	return uuid.New().String()
}

// UUIDStr returns a new UUID string.
func UUIDStr() string {
	return uuidStr()
}

// NameToLastFirst converts "First Last" to "Last, First".
func NameToLastFirst(name string) string {
	parts := strings.Fields(name)
	if len(parts) > 1 {
		return parts[len(parts)-1] + ", " + strings.Join(parts[:len(parts)-1], " ")
	}
	return name
}

// getSortingPrefixes retrieves the sorting prefixes from the database settings.
func getSortingPrefixes(db *sql.DB) []string {
	var prefixes []string
	var valStr string
	_ = db.QueryRow("SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
	if valStr != "" {
		var s struct {
			SortingPrefixes []string `json:"sortingPrefixes"`
		}
		if json.Unmarshal([]byte(valStr), &s) == nil {
			prefixes = s.SortingPrefixes
		}
	}
	if len(prefixes) == 0 {
		prefixes = []string{"the", "a", "an"}
	}
	return prefixes
}

// GetTitleIgnorePrefix returns the title with sorting prefixes removed.
func GetTitleIgnorePrefix(db *sql.DB, title string) string {
	prefixes := getSortingPrefixes(db)
	return getTitleIgnorePrefixGo(title, prefixes)
}

// getTitleIgnorePrefixGo strips common prefixes from titles for sorting.
func getTitleIgnorePrefixGo(title string, prefixes []string) string {
	lower := strings.ToLower(title)
	for _, prefix := range prefixes {
		p := strings.ToLower(prefix) + " "
		if strings.HasPrefix(lower, p) {
			return title[len(p):]
		}
	}
	return title
}

// NullIfEmpty returns nil if the string is empty, otherwise returns a pointer to the string.
func NullIfEmpty(s string) *string {
	return nullIfEmpty(s)
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func timeToDBStr(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05.000 +00:00")
}

func formatEpochMillis(epoch int64) string {
	t := time.Unix(epoch/1000, (epoch%1000)*1000000)
	return timeToDBStr(t)
}

func parseEpochMillis(s string) int64 {
	if s == "" {
		return 0
	}
	// Try integer milliseconds first
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
		return ms
	}
	// Try as SQLite timestamp
	formats := []string{
		"2006-01-02 15:04:05.999 -07:00",
		"2006-01-02 15:04:05.999 +00:00",
		"2006-01-02T15:04:05.999Z",
		"2006-01-02 15:04:05.999",
		"2006-01-02 15:04:05",
	}
	for _, format := range formats {
		t, err := time.Parse(format, s)
		if err == nil {
			return t.UnixMilli()
		}
	}
	return 0
}

func nullIfZero(val int) interface{} {
	if val == 0 {
		return nil
	}
	return val
}

func extractTrackNumberFromFilename(filename string) interface{} {
	re := regexp.MustCompile(`(?i)(?:^|[-_ ])(?:track|tr|t)?\s*(\d{1,3})(?:[-_ ]|$)`)
	match := re.FindStringSubmatch(filename)
	if len(match) > 1 {
		if val, err := strconv.Atoi(match[1]); err == nil {
			return val
		}
	}
	return nil
}
