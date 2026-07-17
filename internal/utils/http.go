package utils

import (
	"net"
	"net/http"
	"strings"
)

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
