package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"audiobookshelf/internal/core"
	log "audiobookshelf/internal/logger"
)

func handleGetLibraryCollections(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !userSess.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}
		initManagers(db)

		collections, err := queryCollectionsForLibrary(r.Context(), db, libraryID)
		if err != nil {
			log.Errorf("[Collection] handleGetLibraryCollections failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		payload := map[string]interface{}{
			"results": collections,
			"total":   len(collections),
			"limit":   0,
			"page":    0,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

func handleGetCollections(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		collections, err := queryCollectionsForLibrary(r.Context(), db, "")
		if err != nil {
			log.Errorf("[Collection] handleGetCollections failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		payload := map[string]interface{}{
			"collections": collections,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

func handleGetCollection(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		c, err := globalPlaylistManager.GetCollection(r.Context(), id)
		if err != nil {
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
				return
			}
			log.Errorf("[Collection] GetCollection failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(c)
	}
}
