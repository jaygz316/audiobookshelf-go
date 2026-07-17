package playlist

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Helper: parse SQLite times
func parseSQLiteTime(s string) (time.Time, error) {
	layouts := []string{
		"2006-01-02 15:04:05.000 +00:00",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02 15:04:05.000000 +00:00",
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("failed to parse sqlite time %q", s)
}

func msToTimeStr(ms int64) string {
	t := time.Unix(ms/1000, (ms%1000)*1000000).UTC()
	return t.Format("2006-01-02 15:04:05.000")
}

func parseMsFromDBStr(s string) int64 {
	t, err := parseSQLiteTime(s)
	if err != nil {
		return 0
	}
	return t.UnixNano() / int64(time.Millisecond)
}

// Helper: check if a table contains a column dynamically
func hasColumn(ctx context.Context, db *sql.DB, tableName, columnName string) bool {
	query := fmt.Sprintf("PRAGMA table_info(%s)", tableName)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name string
		var typeStr string
		var notnull int
		var dfltVal interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltVal, &pk); err == nil {
			if strings.EqualFold(name, columnName) {
				return true
			}
		}
	}
	if err := rows.Err(); err != nil {
		return false
	}
	return false
}
