package scanner

import (
	"database/sql"
	"net/http"

	"audiobookshelf/internal/core"
	log "audiobookshelf/internal/logger"
	isocket "audiobookshelf/internal/socket"
)

// HandleScanLibrary returns an HTTP handler for triggering a library scan.
func HandleScanLibrary(db *sql.DB, libraryID string, socketAuth *isocket.Authority) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" && !userSess.CanUpdate {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var count int
		err := db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM libraries WHERE id = ?", libraryID).Scan(&count)
		if err != nil {
			log.Printf("[Scanner] Database error: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}
		if count == 0 {
			http.Error(w, `{"error": "Library not found"}`, http.StatusNotFound)
			return
		}

		go func() {
			if err := ScanLibrary(db, libraryID, socketAuth); err != nil {
				log.Printf("[Scanner] Scan failed: %v", err)
			}
		}()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}
}
