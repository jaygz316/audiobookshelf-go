package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"audiobookshelf/internal/core"
	log "audiobookshelf/internal/logger"
)

func handleGetLibraryPlaylists(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !userSess.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}
		initManagers(db)

		playlists, err := queryPlaylistsForUserAndLibrary(r.Context(), db, userSess.ID, libraryID)
		if err != nil {
			log.Errorf("[Playlist] handleGetLibraryPlaylists failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		payload := map[string]interface{}{
			"results": playlists,
			"total":   len(playlists),
			"limit":   0,
			"page":    0,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

func handleGetLibraryOPML(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !userSess.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}
		initManagers(db)

		opmlText, err := globalFeedManager.GenerateOPML(r.Context(), userSess.ID, libraryID)
		if err != nil {
			log.Errorf("[Feed] GenerateOPML failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(opmlText))
	}
}

func handleGetPlaylists(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		initManagers(db)

		playlists, err := queryPlaylistsForUserAndLibrary(r.Context(), db, userSess.ID, "")
		if err != nil {
			log.Errorf("[Playlist] handleGetPlaylists failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		payload := map[string]interface{}{
			"playlists": playlists,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

func handleGetPlaylist(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		initManagers(db)

		p, err := globalPlaylistManager.GetPlaylist(r.Context(), id)
		if err != nil {
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
				return
			}
			log.Errorf("[Playlist] GetPlaylist failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if userSess.Type != "root" && userSess.Type != "admin" && userSess.ID != p.UserID {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
	}
}
