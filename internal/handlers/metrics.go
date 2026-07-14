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
	metricHTTPRequests2xx    int64
	metricHTTPRequests3xx    int64
	metricHTTPRequests4xx    int64
	metricHTTPRequests5xx    int64
	metricHTTPRequestsOther  int64
)

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	return w.ResponseWriter.Write(b)
}

// MetricsMiddleware tracks basic HTTP request counts.
func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&metricHTTPActiveRequests, 1)
		defer atomic.AddInt64(&metricHTTPActiveRequests, -1)

		atomic.AddInt64(&metricHTTPRequestsTotal, 1)

		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)

		status := sw.status
		if status == 0 {
			status = 200
		}

		switch {
		case status >= 200 && status < 300:
			atomic.AddInt64(&metricHTTPRequests2xx, 1)
		case status >= 300 && status < 400:
			atomic.AddInt64(&metricHTTPRequests3xx, 1)
		case status >= 400 && status < 500:
			atomic.AddInt64(&metricHTTPRequests4xx, 1)
		case status >= 500 && status < 600:
			atomic.AddInt64(&metricHTTPRequests5xx, 1)
		default:
			atomic.AddInt64(&metricHTTPRequestsOther, 1)
		}
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

		var dbStats sql.DBStats
		hasDBStats := false

		if db != nil {
			_ = db.QueryRow("SELECT COUNT(*) FROM users").Scan(&numUsers)
			_ = db.QueryRow("SELECT COUNT(*) FROM libraries").Scan(&numLibraries)
			_ = db.QueryRow("SELECT COUNT(*) FROM libraryItems").Scan(&numLibraryItems)
			_ = db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&numSessions)

			dbStats = db.Stats()
			hasDBStats = true
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

		_, _ = fmt.Fprintf(w, "# HELP audiobookshelf_http_requests_by_status HTTP requests processed by response status code class.\n")
		_, _ = fmt.Fprintf(w, "# TYPE audiobookshelf_http_requests_by_status counter\n")
		_, _ = fmt.Fprintf(w, "audiobookshelf_http_requests_by_status{code=\"2xx\"} %d\n", atomic.LoadInt64(&metricHTTPRequests2xx))
		_, _ = fmt.Fprintf(w, "audiobookshelf_http_requests_by_status{code=\"3xx\"} %d\n", atomic.LoadInt64(&metricHTTPRequests3xx))
		_, _ = fmt.Fprintf(w, "audiobookshelf_http_requests_by_status{code=\"4xx\"} %d\n", atomic.LoadInt64(&metricHTTPRequests4xx))
		_, _ = fmt.Fprintf(w, "audiobookshelf_http_requests_by_status{code=\"5xx\"} %d\n", atomic.LoadInt64(&metricHTTPRequests5xx))
		_, _ = fmt.Fprintf(w, "audiobookshelf_http_requests_by_status{code=\"other\"} %d\n\n", atomic.LoadInt64(&metricHTTPRequestsOther))

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

		if hasDBStats {
			_, _ = fmt.Fprintf(w, "# HELP audiobookshelf_db_max_open_connections Maximum number of open connections to the database.\n")
			_, _ = fmt.Fprintf(w, "# TYPE audiobookshelf_db_max_open_connections gauge\n")
			_, _ = fmt.Fprintf(w, "audiobookshelf_db_max_open_connections %d\n\n", dbStats.MaxOpenConnections)

			_, _ = fmt.Fprintf(w, "# HELP audiobookshelf_db_open_connections Number of established connections both in use and idle.\n")
			_, _ = fmt.Fprintf(w, "# TYPE audiobookshelf_db_open_connections gauge\n")
			_, _ = fmt.Fprintf(w, "audiobookshelf_db_open_connections %d\n\n", dbStats.OpenConnections)

			_, _ = fmt.Fprintf(w, "# HELP audiobookshelf_db_in_use_connections Number of connections currently in use.\n")
			_, _ = fmt.Fprintf(w, "# TYPE audiobookshelf_db_in_use_connections gauge\n")
			_, _ = fmt.Fprintf(w, "audiobookshelf_db_in_use_connections %d\n\n", dbStats.InUse)

			_, _ = fmt.Fprintf(w, "# HELP audiobookshelf_db_idle_connections Number of idle connections.\n")
			_, _ = fmt.Fprintf(w, "# TYPE audiobookshelf_db_idle_connections gauge\n")
			_, _ = fmt.Fprintf(w, "audiobookshelf_db_idle_connections %d\n\n", dbStats.Idle)

			_, _ = fmt.Fprintf(w, "# HELP audiobookshelf_db_wait_count Total number of connections waited for.\n")
			_, _ = fmt.Fprintf(w, "# TYPE audiobookshelf_db_wait_count counter\n")
			_, _ = fmt.Fprintf(w, "audiobookshelf_db_wait_count %d\n\n", dbStats.WaitCount)

			_, _ = fmt.Fprintf(w, "# HELP audiobookshelf_db_wait_duration_seconds Total time blocked waiting for a new connection.\n")
			_, _ = fmt.Fprintf(w, "# TYPE audiobookshelf_db_wait_duration_seconds counter\n")
			_, _ = fmt.Fprintf(w, "audiobookshelf_db_wait_duration_seconds %f\n\n", dbStats.WaitDuration.Seconds())

			_, _ = fmt.Fprintf(w, "# HELP audiobookshelf_db_max_idle_closed Total number of connections closed due to SetMaxIdleConns.\n")
			_, _ = fmt.Fprintf(w, "# TYPE audiobookshelf_db_max_idle_closed counter\n")
			_, _ = fmt.Fprintf(w, "audiobookshelf_db_max_idle_closed %d\n\n", dbStats.MaxIdleClosed)

			_, _ = fmt.Fprintf(w, "# HELP audiobookshelf_db_max_lifetime_closed Total number of connections closed due to SetConnMaxLifetime.\n")
			_, _ = fmt.Fprintf(w, "# TYPE audiobookshelf_db_max_lifetime_closed counter\n")
			_, _ = fmt.Fprintf(w, "audiobookshelf_db_max_lifetime_closed %d\n\n", dbStats.MaxLifetimeClosed)
		}
	}
}
