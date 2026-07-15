package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

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

func HandleCreateLibrary(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		if user.Type != "admin" && user.Type != "root" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var payload idb.CreateLibraryPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		if payload.Name == "" {
			http.Error(w, `{"error": "Name is required"}`, http.StatusBadRequest)
			return
		}

		for i, f := range payload.Folders {
			fpath := f.FullPath
			if fpath == "" {
				fpath = f.Path
			}
			if fpath == "" {
				http.Error(w, `{"error": "Folder path is required"}`, http.StatusBadRequest)
				return
			}
			absPath, err := filepath.Abs(fpath)
			if err != nil {
				absPath = fpath
			}
			absPath = filepath.ToSlash(absPath)
			if err := os.MkdirAll(absPath, 0755); err != nil {
				log.Errorf("Failed to create folder directory %s: %v", absPath, err)
				http.Error(w, fmt.Sprintf(`{"error": "Invalid folder directory %s"}`, absPath), http.StatusBadRequest)
				return
			}
			payload.Folders[i].Path = absPath
		}

		lib, err := idb.CreateLibrary(db, &payload)
		if err != nil {
			log.Errorf("[Go] Failed to create library: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lib)
	}
}

func HandleUpdateLibrary(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		if user.Type != "admin" && user.Type != "root" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var payload idb.UpdateLibraryPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		if payload.Folders != nil {
			for i, f := range payload.Folders {
				fpath := f.FullPath
				if fpath == "" {
					fpath = f.Path
				}
				if fpath == "" {
					http.Error(w, `{"error": "Folder path is required"}`, http.StatusBadRequest)
					return
				}
				absPath, err := filepath.Abs(fpath)
				if err != nil {
					absPath = fpath
				}
				absPath = filepath.ToSlash(absPath)
				if err := os.MkdirAll(absPath, 0755); err != nil {
					log.Errorf("Failed to create folder directory %s: %v", absPath, err)
					http.Error(w, fmt.Sprintf(`{"error": "Invalid folder directory %s"}`, absPath), http.StatusBadRequest)
					return
				}
				payload.Folders[i].Path = absPath
			}
		}

		lib, err := idb.UpdateLibrary(db, libraryID, &payload)
		if err != nil {
			log.Errorf("[Go] Failed to update library %s: %v", libraryID, err)
			if err.Error() == "library not found" {
				http.Error(w, `{"error": "Library not found"}`, http.StatusNotFound)
			} else {
				http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lib)
	}
}

func HandleDeleteLibrary(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		if user.Type != "admin" && user.Type != "root" && !user.CanDelete {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		lib, err := idb.DeleteLibrary(db, libraryID)
		if err != nil {
			log.Errorf("[Go] Failed to delete library %s: %v", libraryID, err)
			if err.Error() == "library not found" {
				http.Error(w, `{"error": "Library not found"}`, http.StatusNotFound)
			} else {
				http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lib)
	}
}

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
