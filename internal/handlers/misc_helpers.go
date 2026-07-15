package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/logger"
	isocket "audiobookshelf/internal/socket"
)

// EmitLibraryItemEvent emits a library item event to all clients via the socket authority.
func EmitLibraryItemEvent(evt string, item *idb.LibraryItemMinifiedJSON) {
	if item == nil {
		return
	}
	if isocket.GlobalAuth != nil {
		isocket.GlobalAuth.BroadcastToAll(evt, item)
	}
}

// EmitLibraryItemsEvent emits a library items event to all clients via the socket authority.
func EmitLibraryItemsEvent(evt string, item *idb.LibraryItemMinifiedJSON) {
	if item == nil {
		return
	}
	if isocket.GlobalAuth != nil {
		isocket.GlobalAuth.BroadcastToAll(evt, item)
	}
}

// handleGetAdminStatsForYear stub returning mock admin/user listening stats
func handleGetAdminStatsForYear(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		res := map[string]interface{}{
			"totalListeningSessions":    0,
			"totalListeningTime":        0,
			"totalBookListeningTime":    0,
			"totalPodcastListeningTime": 0,
			"topAuthors":                []interface{}{},
			"topGenres":                 []interface{}{},
			"mostListenedNarrator":      nil,
			"mostListenedMonth":         nil,
			"numBooksFinished":          0,
			"numBooksListened":          0,
			"longestAudiobookFinished":  nil,
			"booksWithCovers":           []interface{}{},
			"finishedBooksWithCovers":   []interface{}{},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(res)
	}
}

// handleGetLoggerData stub returning empty logs structure
func handleGetLoggerData(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/logger-data")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"currentDailyLogs": logger.GlobalLogBuffer.Get(),
		})
	}
}

// handleValidateCron validates simple cron expression fields
func handleValidateCron(w http.ResponseWriter, r *http.Request) {
	log.Infof("[Go] POST /api/validate-cron")
	userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
	if !ok || !userSess.IsAdminOrUp() {
		http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
		return
	}

	var body struct {
		Expression string `json:"expression"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
		return
	}

	parts := strings.Fields(body.Expression)
	if len(parts) < 5 || len(parts) > 6 {
		http.Error(w, "Invalid cron expression", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleWatcherUpdate stub for file watcher updates
func handleWatcherUpdate(w http.ResponseWriter, r *http.Request) {
	log.Infof("[Go] POST /api/watcher/update")
	userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
	if !ok || !userSess.IsAdminOrUp() {
		http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusOK)
}
