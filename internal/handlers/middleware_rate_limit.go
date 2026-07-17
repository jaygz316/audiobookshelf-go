package handlers

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	// LoginRateLimiter limits login and initialization attempts (5 requests per minute per IP)
	LoginRateLimiter = NewRateLimiter(5, time.Minute)

	// ShareRateLimiter limits public share requests (30 requests per minute per IP)
	ShareRateLimiter = NewRateLimiter(30, time.Minute)
)

// RateLimiter tracks request rate per IP.
type RateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int           // max requests
	window   time.Duration // time window
	stopChan chan struct{}
}

// NewRateLimiter creates a new RateLimiter.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	limiter := &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
		stopChan: make(chan struct{}),
	}
	go limiter.cleanupLoop()
	return limiter
}

// Close stops the cleanup loop.
func (rl *RateLimiter) Close() {
	close(rl.stopChan)
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.window * 2)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for ip, times := range rl.requests {
				var valid []time.Time
				for _, t := range times {
					if now.Sub(t) < rl.window {
						valid = append(valid, t)
					}
				}
				if len(valid) == 0 {
					delete(rl.requests, ip)
				} else {
					rl.requests[ip] = valid
				}
			}
			rl.mu.Unlock()
		case <-rl.stopChan:
			return
		}
	}
}

// Allow checks if the request is allowed for the given IP.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	times, exists := rl.requests[ip]
	if !exists {
		rl.requests[ip] = []time.Time{now}
		return true
	}

	var active []time.Time
	for _, t := range times {
		if now.Sub(t) < rl.window {
			active = append(active, t)
		}
	}

	if len(active) >= rl.limit {
		rl.requests[ip] = active
		return false
	}

	rl.requests[ip] = append(active, now)
	return true
}

// RateLimitMiddleware returns a middleware that rate limits requests.
func RateLimitMiddleware(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.Header.Get("X-Forwarded-For")
			if ip == "" {
				ip = r.Header.Get("X-Real-IP")
			}
			if ip == "" {
				idx := strings.LastIndex(r.RemoteAddr, ":")
				if idx != -1 {
					ip = r.RemoteAddr[:idx]
				} else {
					ip = r.RemoteAddr
				}
			}

			// Clean IPv6 brackets if RemoteAddr has them
			ip = strings.TrimPrefix(ip, "[")
			ip = strings.TrimSuffix(ip, "]")

			if !limiter.Allow(ip) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error": "Too many requests. Please try again later."}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
