package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

// handleGetLibraryFilterData resolves GET /api/libraries/{id}/filter-data
func handleGetLibraryFilterData(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		fd, err := idb.GetLibraryFilterDataGo(db, libraryID)
		if err != nil {
			log.Errorf("[Library getFilterData] Error: %v", err)
			http.Error(w, `{"error": "Failed to load filter data"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fd)
	}
}

// handleGetLibraryStats resolves GET /api/libraries/{id}/stats
func handleGetLibraryStats(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		lib, err := idb.GetLibraryByID(db, libraryID)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, `{"error": "Library not found"}`, http.StatusNotFound)
			} else {
				log.Errorf("[idb.LibraryStats] Failed to get library %s: %v", libraryID, err)
				http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			}
			return
		}
		if lib == nil {
			http.Error(w, "Library not found", http.StatusNotFound)
			return
		}

		var stats *idb.LibraryStats
		if lib.MediaType == "book" {
			stats, err = idb.GetBookLibraryStats(db, libraryID)
		} else {
			stats, err = idb.GetPodcastLibraryStats(db, libraryID)
		}
		if err != nil {
			log.Errorf("[idb.LibraryStats] Failed to get stats for library %s: %v", libraryID, err)
			http.Error(w, `{"error": "Failed to load library stats"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stats)
	}
}
