package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
)

// handleDeleteLibraryItemByID resolves DELETE /api/items/{id}
func handleDeleteLibraryItemByID(db *sql.DB, itemID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] DELETE /api/items/%s", itemID)

		if strings.Contains(itemID, "..") || strings.Contains(itemID, "/") || strings.Contains(itemID, "\\") {
			http.Error(w, `{"error": "Invalid item ID"}`, http.StatusBadRequest)
			return
		}

		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		if user.Type != "root" && user.Type != "admin" && !user.CanDelete {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		// Verify library item exists and get mediaId & mediaType
		var mediaID, mediaType string
		err := db.QueryRow("SELECT mediaId, mediaType FROM libraryItems WHERE id = ?", itemID).Scan(&mediaID, &mediaType)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			log.Errorf("[Delete Item] Failed to begin transaction: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		// Delete from playlistMediaItems
		_, _ = tx.Exec("DELETE FROM playlistMediaItems WHERE mediaItemId = ?", itemID)

		// Delete from mediaProgresses
		_, _ = tx.Exec("DELETE FROM mediaProgresses WHERE mediaItemId = ?", itemID)

		// Delete from libraryItems
		_, err = tx.Exec("DELETE FROM libraryItems WHERE id = ?", itemID)
		if err != nil {
			log.Errorf("[Delete Item] Failed to delete library item: %v", err)
			http.Error(w, `{"error": "Database Error"}`, http.StatusInternalServerError)
			return
		}

		// Clean up book/podcast if they are no longer referenced in libraryItems
		if mediaType == "book" {
			_, _ = tx.Exec("DELETE FROM bookAuthors WHERE bookId = ?", mediaID)
			_, _ = tx.Exec("DELETE FROM bookSeries WHERE bookId = ?", mediaID)
			_, _ = tx.Exec("DELETE FROM books WHERE id = ? AND id NOT IN (SELECT mediaId FROM libraryItems WHERE mediaType = 'book')", mediaID)
		} else if mediaType == "podcast" {
			_, _ = tx.Exec("DELETE FROM podcastEpisodes WHERE podcastId = ?", mediaID)
			_, _ = tx.Exec("DELETE FROM podcasts WHERE id = ? AND id NOT IN (SELECT mediaId FROM libraryItems WHERE mediaType = 'podcast')", mediaID)
		}

		if err := tx.Commit(); err != nil {
			log.Errorf("[Delete Item] Failed to commit transaction: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success": true}`))
	}
}
