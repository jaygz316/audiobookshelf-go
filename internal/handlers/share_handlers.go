package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"audiobookshelf/internal/share"

	"golang.org/x/crypto/bcrypt"

	"audiobookshelf/internal/core"
)

func handleCreateShare(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		initManagers(db)

		var req struct {
			Slug           string `json:"slug"`
			ExpiresAt      int64  `json:"expiresAt"`
			MediaItemID    string `json:"mediaItemId"`
			MediaItemType  string `json:"mediaItemType"`
			IsDownloadable bool   `json:"isDownloadable"`
			Password       string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var libraryItemID string
		err := db.QueryRowContext(r.Context(), "SELECT id FROM libraryItems WHERE mediaId = ?", req.MediaItemID).Scan(&libraryItemID)
		if err != nil {
			err = db.QueryRowContext(r.Context(), "SELECT id FROM libraryItems WHERE id = ?", req.MediaItemID).Scan(&libraryItemID)
			if err != nil {
				log.Printf("[Share] Failed to find libraryItem for mediaItemId %s: %v", req.MediaItemID, err)
				http.Error(w, "Media item not found", http.StatusNotFound)
				return
			}
		}

		var expiresTime time.Time
		if req.ExpiresAt > 0 {
			expiresTime = time.Unix(req.ExpiresAt/1000, (req.ExpiresAt%1000)*1000000)
		}

		var pash string
		if req.Password != "" {
			hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
			if err == nil {
				pash = string(hash)
			}
		}

		s := &share.ShareLink{
			ID:             req.Slug,
			LibraryItemID:  libraryItemID,
			CreatedBy:      userSess.ID,
			ExpiresAt:      expiresTime,
			IsDownloadable: req.IsDownloadable,
			PasswordHash:   pash,
		}

		if err := globalShareManager.CreateShare(r.Context(), s); err != nil {
			log.Printf("[Share] CreateShare failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		resPayload := map[string]interface{}{
			"id":             s.ID,
			"slug":           s.ID,
			"libraryItemId":  s.LibraryItemID,
			"mediaItemId":    req.MediaItemID,
			"mediaItemType":  req.MediaItemType,
			"userId":         s.CreatedBy,
			"expiresAt":      nil,
			"isDownloadable": s.IsDownloadable,
			"createdAt":      s.CreatedAt.UnixNano() / int64(time.Millisecond),
			"updatedAt":      s.UpdatedAt.UnixNano() / int64(time.Millisecond),
		}
		if !s.ExpiresAt.IsZero() {
			resPayload["expiresAt"] = req.ExpiresAt
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resPayload)
	}
}

func handleDeleteShare(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		if err := globalShareManager.DeleteShare(r.Context(), id); err != nil {
			log.Printf("[Share] DeleteShare failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func handleGetShares(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		list, err := globalShareManager.GetShares(r.Context())
		if err != nil {
			log.Printf("[Share] GetShares failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Hydrate with audiobook/podcast media titles if possible!
		type HydratedShare struct {
			*share.ShareLink
			MediaTitle string `json:"mediaTitle"`
			MediaType  string `json:"mediaType"`
		}

		hydratedList := make([]HydratedShare, 0, len(list))
		for _, s := range list {
			var title, mediaType string
			err := db.QueryRowContext(r.Context(), "SELECT title, mediaType FROM libraryItems WHERE id = ?", s.LibraryItemID).Scan(&title, &mediaType)
			if err != nil {
				title = "Unknown Item"
			}
			hydratedList = append(hydratedList, HydratedShare{
				ShareLink:  s,
				MediaTitle: title,
				MediaType:  mediaType,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(hydratedList)
	}
}
