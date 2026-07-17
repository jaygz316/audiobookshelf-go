package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	log "audiobookshelf/internal/logger"
	isocket "audiobookshelf/internal/socket"
)

type BatchUpdateMediaPayload struct {
	Title          *string   `json:"title"`
	Subtitle       *string   `json:"subtitle"`
	Authors        *[]string `json:"authors"`
	Narrators      *[]string `json:"narrators"`
	SeriesName     *string   `json:"seriesName"`
	SeriesSequence *string   `json:"seriesSequence"`
	Publisher      *string   `json:"publisher"`
	PublishedYear  *string   `json:"publishedYear"`
	PublishedDate  *string   `json:"publishedDate"`
	Description    *string   `json:"description"`
	Isbn           *string   `json:"isbn"`
	Asin           *string   `json:"asin"`
	Language       *string   `json:"language"`
	Explicit       *bool     `json:"explicit"`
	Abridged       *bool     `json:"abridged"`
	Tags           *[]string `json:"tags"`
	Genres         *[]string `json:"genres"`
}

type BatchUpdateItem struct {
	ID           string                  `json:"id"`
	MediaPayload BatchUpdateMediaPayload `json:"mediaPayload"`
}

type updatedItemInfo struct {
	itemID    string
	mediaID   string
	mediaType string
	payload   BatchUpdateMediaPayload
}

func handleBatchUpdateLibraryItems(db *sql.DB, cfg *core.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST /api/items/batch/update")

		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		if user.Type != "root" && user.Type != "admin" && !user.CanUpdate {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var payload []BatchUpdateItem
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		nowStr := time.Now().Format("2006-01-02 15:04:05.000")
		prefixes := idb.GetSortingPrefixes(db)

		var updatedItems []updatedItemInfo

		for _, item := range payload {
			var mediaID, mediaType, libraryID string
			err = tx.QueryRow("SELECT COALESCE(mediaId, ''), COALESCE(mediaType, ''), COALESCE(libraryId, '') FROM libraryItems WHERE id = ?", item.ID).Scan(&mediaID, &mediaType, &libraryID)
			if err != nil {
				log.Warnf("[Go] Batch edit: item %s not found", item.ID)
				continue
			}

			if mediaType == "book" {
				updated, err := batchUpdateBook(tx, item.ID, mediaID, libraryID, item.MediaPayload, prefixes, nowStr)
				if err != nil {
					return
				}
				if !updated {
					continue
				}
			} else if mediaType == "podcast" {
				updated, err := batchUpdatePodcast(tx, item.ID, mediaID, item.MediaPayload, prefixes, nowStr)
				if err != nil {
					return
				}
				if !updated {
					continue
				}
			}

			updatedItems = append(updatedItems, updatedItemInfo{
				itemID:    item.ID,
				mediaID:   mediaID,
				mediaType: mediaType,
				payload:   item.MediaPayload,
			})
		}

		err = tx.Commit()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		srvSettings, srvErr := idb.GetServerSettings(db)
		for _, info := range updatedItems {
			if srvErr == nil {
				_ = writeBatchMetadata(db, info.itemID, info.mediaID, info.mediaType, srvSettings)
			}

			if isocket.GlobalAuth != nil {
				if minItem, err := idb.GetLibraryItemMinifiedByID(db, info.itemID); err == nil {
					EmitLibraryItemEvent("item_updated", minItem)
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true}`))
	}
}
