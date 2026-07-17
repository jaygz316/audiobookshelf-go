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

func handleCreateCollection(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}
		initManagers(db)

		var req struct {
			Name        string      `json:"name"`
			Description string      `json:"description"`
			LibraryID   string      `json:"libraryId"`
			Books       []string    `json:"books"`
			IsSmart     bool        `json:"isSmart"`
			Rules       interface{} `json:"rules"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var rulesStr string
		if req.Rules != nil {
			switch v := req.Rules.(type) {
			case string:
				rulesStr = v
			default:
				bytes, err := json.Marshal(v)
				if err == nil {
					rulesStr = string(bytes)
				}
			}
		}

		c := &playlist.Collection{
			ID:          uuid.New().String(),
			Name:        req.Name,
			Description: req.Description,
			LibraryID:   req.LibraryID,
			ItemIDs:     req.Books,
			IsSmart:     req.IsSmart,
			Rules:       rulesStr,
		}

		if err := globalPlaylistManager.CreateCollection(r.Context(), c); err != nil {
			log.Errorf("[Collection] Create failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(c)
	}
}

func handleUpdateCollection(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}
		initManagers(db)

		c, err := globalPlaylistManager.GetCollection(r.Context(), id)
		if err != nil {
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var req struct {
			Name        string      `json:"name"`
			Description string      `json:"description"`
			LibraryID   string      `json:"libraryId"`
			Books       []string    `json:"books"`
			IsSmart     *bool       `json:"isSmart"`
			Rules       interface{} `json:"rules"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.Name != "" {
			c.Name = req.Name
		}
		if req.Description != "" {
			c.Description = req.Description
		}
		if req.LibraryID != "" {
			c.LibraryID = req.LibraryID
		}
		if req.Books != nil {
			c.ItemIDs = req.Books
		}
		if req.IsSmart != nil {
			c.IsSmart = *req.IsSmart
		}
		if req.Rules != nil {
			var rulesStr string
			switch v := req.Rules.(type) {
			case string:
				rulesStr = v
			default:
				bytes, err := json.Marshal(v)
				if err == nil {
					rulesStr = string(bytes)
				}
			}
			c.Rules = rulesStr
		}

		if err := globalPlaylistManager.UpdateCollection(r.Context(), c); err != nil {
			log.Errorf("[Collection] Update failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(c)
	}
}

func handleDeleteCollection(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}
		initManagers(db)

		if err := globalPlaylistManager.DeleteCollection(r.Context(), id); err != nil {
			log.Errorf("[Collection] Delete failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}
}
