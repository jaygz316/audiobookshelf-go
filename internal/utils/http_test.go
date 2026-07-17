package utils

import (
	"net/http/httptest"
	"testing"
)

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		expected   string
	}{
		{
			name:       "no headers, just remote addr",
			headers:    nil,
			remoteAddr: "192.168.1.50:56123",
			expected:   "192.168.1.50",
		},
		{
			name: "X-Forwarded-For single IP",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.195",
			},
			remoteAddr: "127.0.0.1:56123",
			expected:   "203.0.113.195",
		},
		{
			name: "X-Forwarded-For multiple IPs",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.195, 70.41.3.18, 150.172.238.178",
			},
			remoteAddr: "127.0.0.1:56123",
			expected:   "203.0.113.195",
		},
		{
			name: "X-Real-IP fallback",
			headers: map[string]string{
				"X-Real-IP": "198.51.100.1",
			},
			remoteAddr: "127.0.0.1:56123",
			expected:   "198.51.100.1",
		},
		{
			name: "X-Forwarded-For takes precedence over X-Real-IP",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.195",
				"X-Real-IP":       "198.51.100.1",
			},
			remoteAddr: "127.0.0.1:56123",
			expected:   "203.0.113.195",
		},
		{
			name:       "Remote Addr IPv6 without port",
			headers:    nil,
			remoteAddr: "::1",
			expected:   "::1",
		},
		{
			name:       "Remote Addr IPv6 with port",
			headers:    nil,
			remoteAddr: "[::1]:56123",
			expected:   "::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com/", nil)
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			got := GetClientIP(req)
			if got != tt.expected {
				t.Errorf("GetClientIP() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestTrimAPIPath(t *testing.T) {
	tests := []struct {
		path     string
		segment  string
		expected string
	}{
		{"api/v1/libraries", "api/v1", "/libraries"},
		{"/api/v1/libraries", "/api/v1", "/libraries"},
		{"/api/v1/libraries", "notfound", "/api/v1/libraries"},
		{"/api/v1/libraries", "/api/v1/libraries", ""},
		{"", "segment", ""},
	}

	for _, tt := range tests {
		got := TrimAPIPath(tt.path, tt.segment)
		if got != tt.expected {
			t.Errorf("TrimAPIPath(%q, %q) = %q, want %q", tt.path, tt.segment, got, tt.expected)
		}
	}
}
