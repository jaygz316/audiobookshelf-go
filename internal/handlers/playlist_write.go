package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"audiobookshelf/internal/core"
	log "audiobookshelf/internal/logger"
	"audiobookshelf/internal/playlist"

	"github.com/google/uuid"
)

func handleCreatePlaylist(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		initManagers(db)

		var req struct {
			Name  string   `json:"name"`
			Items []string `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		p := &playlist.Playlist{
			ID:      uuid.New().String(),
			UserID:  userSess.ID,
			Name:    req.Name,
			ItemIDs: req.Items,
		}

		if err := globalPlaylistManager.CreatePlaylist(r.Context(), p); err != nil {
			log.Errorf("[Playlist] Create failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(p)
	}
}

func handleUpdatePlaylist(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		initManagers(db)

		p, err := globalPlaylistManager.GetPlaylist(r.Context(), id)
		if err != nil {
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if userSess.Type != "root" && userSess.Type != "admin" && userSess.ID != p.UserID {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var req struct {
			Name  string   `json:"name"`
			Items []string `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.Name != "" {
			p.Name = req.Name
		}
		if req.Items != nil {
			p.ItemIDs = req.Items
		}

		if err := globalPlaylistManager.UpdatePlaylist(r.Context(), p); err != nil {
			log.Errorf("[Playlist] Update failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
	}
}

func handleDeletePlaylist(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		initManagers(db)

		p, err := globalPlaylistManager.GetPlaylist(r.Context(), id)
		if err != nil {
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if userSess.Type != "root" && userSess.Type != "admin" && userSess.ID != p.UserID {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		if err := globalPlaylistManager.DeletePlaylist(r.Context(), id); err != nil {
			log.Errorf("[Playlist] Delete failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}
}
