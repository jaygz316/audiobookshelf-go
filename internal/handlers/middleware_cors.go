package handlers

import (
	idb "audiobookshelf/internal/db"
	"database/sql"
	"net/http"
	"strings"
)

type corsResponseWriter struct {
	http.ResponseWriter
	allowedOrigin string
	headersSet    bool
}

func (w *corsResponseWriter) setCORSHeaders() {
	if w.headersSet {
		return
	}
	h := w.ResponseWriter.Header()
	if w.allowedOrigin != "" {
		h.Set("Access-Control-Allow-Origin", w.allowedOrigin)
		h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Origin, Accept")
		h.Set("Access-Control-Allow-Credentials", "true")
	} else {
		h.Del("Access-Control-Allow-Origin")
		h.Del("Access-Control-Allow-Methods")
		h.Del("Access-Control-Allow-Headers")
		h.Del("Access-Control-Allow-Credentials")
	}
	w.headersSet = true
}

func (w *corsResponseWriter) WriteHeader(statusCode int) {
	w.setCORSHeaders()
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *corsResponseWriter) Write(b []byte) (int, error) {
	w.setCORSHeaders()
	return w.ResponseWriter.Write(b)
}

// CORSMiddleware handles CORS requests and pre-flight OPTIONS requests.
func CORSMiddleware(db *sql.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		var allowedOrigin string

		if origin != "" {
			settings, err := idb.GetServerSettings(db)
			if err == nil && settings != nil && settings.AllowedCorsOrigins != "" {
				origins := strings.Split(settings.AllowedCorsOrigins, ",")
				for _, o := range origins {
					if strings.TrimSpace(o) == origin {
						allowedOrigin = origin
						break
					}
				}
			}
		}

		if r.Method == "OPTIONS" {
			if allowedOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Origin, Accept")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		writerWrapper := &corsResponseWriter{
			ResponseWriter: w,
			allowedOrigin:  allowedOrigin,
		}

		next.ServeHTTP(writerWrapper, r)
	})
}
