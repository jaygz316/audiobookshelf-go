package utils

import (
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// ReplaceInJSONArray replaces oldVal with newVal in a JSON array string.
func ReplaceInJSONArray(jsonStr sql.NullString, oldVal, newVal string) (string, bool) {
	if !jsonStr.Valid || jsonStr.String == "" || jsonStr.String == "null" {
		return "[]", false
	}
	var arr []string
	if err := json.Unmarshal([]byte(jsonStr.String), &arr); err != nil {
		return jsonStr.String, false
	}
	found := false
	newArr := []string{}
	for _, val := range arr {
		if val == oldVal {
			found = true
			alreadyHasNew := false
			for _, v := range arr {
				if v == newVal {
					alreadyHasNew = true
					break
				}
			}
			if !alreadyHasNew {
				newArr = append(newArr, newVal)
			}
		} else {
			newArr = append(newArr, val)
		}
	}
	if !found {
		return jsonStr.String, false
	}
	res, _ := json.Marshal(newArr)
	return string(res), true
}

// RemoveFromJSONArray removes valToRemove from a JSON array string.
func RemoveFromJSONArray(jsonStr sql.NullString, valToRemove string) (string, bool) {
	if !jsonStr.Valid || jsonStr.String == "" || jsonStr.String == "null" {
		return "[]", false
	}
	var arr []string
	if err := json.Unmarshal([]byte(jsonStr.String), &arr); err != nil {
		return jsonStr.String, false
	}
	found := false
	newArr := []string{}
	for _, val := range arr {
		if val == valToRemove {
			found = true
		} else {
			newArr = append(newArr, val)
		}
	}
	if !found {
		return jsonStr.String, false
	}
	res, _ := json.Marshal(newArr)
	return string(res), true
}

// IsSameOrSubPath checks if childPath is the same as or nested under parentPath.
func IsSameOrSubPath(parentPath, childPath string) bool {
	parentPath = filepath.Clean(parentPath)
	childPath = filepath.Clean(childPath)
	if parentPath == childPath {
		return true
	}
	rel, err := filepath.Rel(parentPath, childPath)
	if err != nil {
		return false
	}
	if rel == "" || rel == "." {
		return true
	}
	return !strings.HasPrefix(rel, "..")
}

// NameToLastFirst converts "First Last" to "Last, First".
func NameToLastFirst(name string) string {
	parts := strings.Fields(name)
	if len(parts) > 1 {
		return parts[len(parts)-1] + ", " + strings.Join(parts[:len(parts)-1], " ")
	}
	return name
}

// NullIfEmpty returns nil if s is empty, otherwise returns a pointer to s.
func NullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// UUIDStr returns a new UUID string.
func UUIDStr() string {
	return uuid.New().String()
}

// TrimAPIPath extracts the subpath after the specified API segment, in a router base path agnostic manner.
func TrimAPIPath(path, segment string) string {
	if idx := strings.Index(path, segment); idx != -1 {
		return path[idx+len(segment):]
	}
	return strings.TrimPrefix(path, segment)
}

// GetClientIP retrieves the real client IP address from the request, respecting reverse proxy headers.
func GetClientIP(r *http.Request) string {
	var ip string
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip = strings.TrimSpace(parts[0])
		}
	}
	if ip == "" {
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			ip = strings.TrimSpace(xri)
		}
	}
	if ip == "" {
		ip = r.RemoteAddr
	}
	if host, _, err := net.SplitHostPort(ip); err == nil {
		return host
	}
	return ip
}


