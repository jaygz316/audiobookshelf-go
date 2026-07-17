package handlers

import (
	log "audiobookshelf/internal/logger"
	"net/http"
	"strings"
)

// LoggingMiddleware logs incoming HTTP requests with sanitized headers.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Info("[HTTP] Request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
			"userAgent", r.UserAgent(),
		)

		sanitizedHeaders := make(http.Header)
		for k, v := range r.Header {
			lowerK := strings.ToLower(k)
			if strings.Contains(lowerK, "token") ||
				strings.Contains(lowerK, "key") ||
				strings.Contains(lowerK, "secret") ||
				strings.Contains(lowerK, "auth") ||
				strings.Contains(lowerK, "cookie") ||
				strings.Contains(lowerK, "password") {
				sanitizedHeaders[k] = []string{"[REDACTED]"}
			} else {
				sanitizedHeaders[k] = v
			}
		}
		log.Info("[HTTP] Request Headers", "headers", sanitizedHeaders)
		next.ServeHTTP(w, r)
	})
}
