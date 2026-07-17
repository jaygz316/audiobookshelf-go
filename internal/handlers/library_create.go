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

// HandleCreateLibrary resolves POST /api/libraries
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
			absPath, err := cleanAndCreateFolder(f.FullPath, f.Path)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusBadRequest)
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
