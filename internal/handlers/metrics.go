package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"runtime"
	"sync/atomic"
)

var (
	metricHTTPRequestsTotal  int64
	metricHTTPActiveRequests int64
)

// MetricsMiddleware tracks basic HTTP request counts.
func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&metricHTTPActiveRequests, 1)
		defer atomic.AddInt64(&metricHTTPActiveRequests, -1)

		atomic.AddInt64(&metricHTTPRequestsTotal, 1)
		next.ServeHTTP(w, r)
	})
}

// handleMetrics serves a Prometheus-compatible metrics page.
func handleMetrics(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Gather runtime memory/goroutine metrics
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		numGoroutine := runtime.NumGoroutine()

		// Gather application entity counts from database
		var numUsers int
		var numLibraries int
		var numLibraryItems int
		var numSessions int

		if db != nil {
			_ = db.QueryRow("SELECT COUNT(*) FROM users").Scan(&numUsers)
			_ = db.QueryRow("SELECT COUNT(*) FROM libraries").Scan(&numLibraries)
			_ = db.QueryRow("SELECT COUNT(*) FROM libraryItems").Scan(&numLibraryItems)
			_ = db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&numSessions)
		}

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		// Write standard Prometheus format metrics
		_, _ = fmt.Fprintf(w, "# HELP go_goroutines Number of goroutines that currently exist.\n")
		_, _ = fmt.Fprintf(w, "# TYPE go_goroutines gauge\n")
		_, _ = fmt.Fprintf(w, "go_goroutines %d\n\n", numGoroutine)

		_, _ = fmt.Fprintf(w, "# HELP go_memstats_alloc_bytes Number of bytes allocated and still in use.\n")
		_, _ = fmt.Fprintf(w, "# TYPE go_memstats_alloc_bytes gauge\n")
		_, _ = fmt.Fprintf(w, "go_memstats_alloc_bytes %d\n\n", m.Alloc)

		_, _ = fmt.Fprintf(w, "# HELP go_memstats_sys_bytes Number of bytes obtained from system.\n")
		_, _ = fmt.Fprintf(w, "# TYPE go_memstats_sys_bytes gauge\n")
		_, _ = fmt.Fprintf(w, "go_memstats_sys_bytes %d\n\n", m.Sys)

		_, _ = fmt.Fprintf(w, "# HELP audiobookshelf_http_requests_total Total number of HTTP requests processed.\n")
		_, _ = fmt.Fprintf(w, "# TYPE audiobookshelf_http_requests_total counter\n")
		_, _ = fmt.Fprintf(w, "audiobookshelf_http_requests_total %d\n\n", atomic.LoadInt64(&metricHTTPRequestsTotal))

		_, _ = fmt.Fprintf(w, "# HELP audiobookshelf_http_active_requests Number of currently active HTTP requests.\n")
		_, _ = fmt.Fprintf(w, "# TYPE audiobookshelf_http_active_requests gauge\n")
		_, _ = fmt.Fprintf(w, "audiobookshelf_http_active_requests %d\n\n", atomic.LoadInt64(&metricHTTPActiveRequests))

		_, _ = fmt.Fprintf(w, "# HELP audiobookshelf_users_total Total number of users.\n")
		_, _ = fmt.Fprintf(w, "# TYPE audiobookshelf_users_total gauge\n")
		_, _ = fmt.Fprintf(w, "audiobookshelf_users_total %d\n\n", numUsers)

		_, _ = fmt.Fprintf(w, "# HELP audiobookshelf_libraries_total Total number of libraries.\n")
		_, _ = fmt.Fprintf(w, "# TYPE audiobookshelf_libraries_total gauge\n")
		_, _ = fmt.Fprintf(w, "audiobookshelf_libraries_total %d\n\n", numLibraries)

		_, _ = fmt.Fprintf(w, "# HELP audiobookshelf_library_items_total Total number of library items.\n")
		_, _ = fmt.Fprintf(w, "# TYPE audiobookshelf_library_items_total gauge\n")
		_, _ = fmt.Fprintf(w, "audiobookshelf_library_items_total %d\n\n", numLibraryItems)

		_, _ = fmt.Fprintf(w, "# HELP audiobookshelf_sessions_total Total number of active user sessions.\n")
		_, _ = fmt.Fprintf(w, "# TYPE audiobookshelf_sessions_total gauge\n")
		_, _ = fmt.Fprintf(w, "audiobookshelf_sessions_total %d\n\n", numSessions)
	}
}
