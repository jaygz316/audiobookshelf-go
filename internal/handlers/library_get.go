package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

// HandleGetLibraries resolves GET /api/libraries
func HandleGetLibraries(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		libs, err := idb.GetLibraries(db)
		if err != nil {
			log.Errorf("[Go] Failed to get libraries: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		var filteredLibs []*idb.LibraryJSON = []*idb.LibraryJSON{}
		includeStats := strings.Contains(r.URL.Query().Get("include"), "stats")

		for _, lib := range libs {
			if user.CanAccessLibrary(lib.ID) {
				if includeStats {
					var stats *idb.LibraryStats
					var err error
					if lib.MediaType == "book" {
						stats, err = idb.GetBookLibraryStats(db, lib.ID)
					} else if lib.MediaType == "podcast" {
						stats, err = idb.GetPodcastLibraryStats(db, lib.ID)
					}
					if err == nil {
						lib.Stats = stats
					}
				}
				filteredLibs = append(filteredLibs, lib)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"libraries": filteredLibs,
		})
	}
}

// HandleGetLibraryByID resolves GET /api/libraries/{id}
func HandleGetLibraryByID(db *sql.DB, libraryID string) http.HandlerFunc {
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

		if strings.Contains(r.URL.RawQuery, "include=filterdata") {
			fd, err := idb.GetLibraryFilterDataGo(db, libraryID)
			if err != nil {
				log.Errorf("[Library getFilterData] Error: %v", err)
				http.Error(w, `{"error": "Failed to load filter data"}`, http.StatusInternalServerError)
				return
			}
			lib, err := idb.GetLibraryByID(db, libraryID)
			if err != nil || lib == nil {
				http.Error(w, `{"error": "Library not found"}`, http.StatusNotFound)
				return
			}
			playlists, err := queryPlaylistsForUserAndLibrary(r.Context(), db, user.ID, libraryID)
			numPlaylists := 0
			if err == nil {
				numPlaylists = len(playlists)
			}
			responsePayload := map[string]interface{}{
				"library":          lib,
				"filterdata":       fd,
				"issues":           fd.NumIssues,
				"numUserPlaylists": numPlaylists,
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(responsePayload)
			return
		}

		lib, err := idb.GetLibraryByID(db, libraryID)
		if err != nil {
			log.Errorf("[Go] Failed to get library %s: %v", libraryID, err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		if lib == nil {
			http.Error(w, "Library not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lib)
	}
}
