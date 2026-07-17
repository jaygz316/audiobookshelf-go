package db

import (
	"encoding/json"
	"strconv"
	"time"
)

func ParseTimeStr(s string) int64 {
	if s == "" {
		return 0
	}
	// Try parsing as raw integer first (milliseconds timestamp)
	if val, err := strconv.ParseInt(s, 10, 64); err == nil {
		return val
	}
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t.UnixNano() / int64(time.Millisecond)
	}
	t2, err2 := time.Parse("2006-01-02 15:04:05.000 +00:00", s)
	if err2 == nil {
		return t2.UnixNano() / int64(time.Millisecond)
	}
	t3, err3 := time.Parse("2006-01-02 15:04:05", s)
	if err3 == nil {
		return t3.UnixNano() / int64(time.Millisecond)
	}
	return 0
}

func TimeToDBStr(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05.000 +00:00")
}

// GetDefaultPermissionsForUserType maps type to default permissions JSON
func GetDefaultPermissionsForUserType(userType string) string {
	isAccess := false
	if userType == "root" || userType == "admin" {
		isAccess = true
	}
	perms := map[string]interface{}{
		"download":                  true,
		"accessExplicitContent":     isAccess,
		"accessAllLibraries":        true,
		"librariesAccessible":       []string{},
		"accessAllTags":             true,
		"itemTagsSelected":          []string{},
		"selectedTagsNotAccessible": false,
	}
	bytes, _ := json.Marshal(perms)
	return string(bytes)
}
