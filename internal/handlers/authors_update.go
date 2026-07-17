package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	isocket "audiobookshelf/internal/socket"
	"audiobookshelf/internal/utils"
)

// handleUpdateAuthor resolves PATCH /api/authors/{id}
func handleUpdateAuthor(db *sql.DB, authorID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] PATCH /api/authors/%s", authorID)

		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)
		if user.Type != "root" && user.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var payload struct {
			Name        string `json:"name"`
			LastFirst   string `json:"lastFirst"`
			Asin        string `json:"asin"`
			Description string `json:"description"`
		}

		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		booksToUpdate, err := updateAuthorBooksCacheAndMetadata(db, authorID, payload.Name, payload.LastFirst, payload.Asin, payload.Description)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Perform post-commit disk writes and websocket events
		writeAuthorMetadataAndEmit(db, booksToUpdate)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true}`))
	}
}

func writeAuthorMetadataAndEmit(db *sql.DB, booksToUpdate []BookUpdate) {
	for _, b := range booksToUpdate {
		if b.metadataPath != "" && utils.IsSafeFilePath(db, MetadataPath, b.metadataPath) {
			if _, err := os.Stat(b.metadataPath); err == nil {
				var metadata map[string]interface{}
				if mBytes, err := os.ReadFile(b.metadataPath); err == nil {
					if json.Unmarshal(mBytes, &metadata) == nil {
						metadata["authors"] = b.authorNames
						if mJSON, err := json.MarshalIndent(metadata, "", "  "); err == nil {
							_ = os.WriteFile(b.metadataPath, mJSON, 0644)
						}
					}
				}
			}
		}

		// Emit real-time update
		if isocket.GlobalAuth != nil && b.itemID != "" {
			if minItem, err := idb.GetLibraryItemMinifiedByID(db, b.itemID); err == nil {
				EmitLibraryItemEvent("item_updated", minItem)
			}
		}
	}
}
