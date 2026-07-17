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

// Shelf represents a shelf row on the personalized page.
type Shelf struct {
	ID             string                         `json:"id"`
	Label          string                         `json:"label"`
	LabelStringKey string                         `json:"labelStringKey"`
	Type           string                         `json:"type"`
	Entities       []*idb.LibraryItemMinifiedJSON `json:"entities"`
}

func HandleGetLibraryPersonalized(db *sql.DB, libraryID string) http.HandlerFunc {
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
			log.Errorf("[Go] Failed to get library %s: %v", libraryID, err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		if lib == nil {
			http.Error(w, "Library not found", http.StatusNotFound)
			return
		}

		q := r.URL.Query()
		limitVal := 20
		if q.Get("limit") != "" {
			fmt.Sscanf(q.Get("limit"), "%d", &limitVal)
		}

		var shelves []Shelf

		// 1. Fetch in-progress items
		progressShelves, err := fetchProgressShelves(db, libraryID, user, limitVal, lib.MediaType)
		if err == nil && len(progressShelves) > 0 {
			shelves = append(shelves, progressShelves...)
		}

		// 1.5. Fetch continue series items (books only)
		if lib.MediaType == "book" {
			seriesShelf, err := fetchContinueSeriesShelf(db, libraryID, user, limitVal)
			if err == nil && seriesShelf != nil {
				shelves = append(shelves, *seriesShelf)
			}
		}

		// 2. Fetch recently added items
		recentShelf, err := fetchRecentlyAddedShelf(db, libraryID, user, limitVal, lib.MediaType)
		if err == nil && recentShelf != nil {
			shelves = append(shelves, *recentShelf)
		}

		// 3. Fetch recent series (books only)
		if lib.MediaType == "book" {
			rsShelf, err := fetchRecentSeriesShelf(db, libraryID, user, limitVal)
			if err == nil && rsShelf != nil {
				shelves = append(shelves, *rsShelf)
			}
		}

		// 4. Fetch discover shelf
		discShelf, err := fetchDiscoverShelf(db, libraryID, user, limitVal, lib.MediaType)
		if err == nil && discShelf != nil {
			shelves = append(shelves, *discShelf)
		}

		// 5. Fetch finished shelves (Listen Again, Read Again)
		finishedShelves, err := fetchFinishedShelves(db, libraryID, user, limitVal, lib.MediaType)
		if err == nil && len(finishedShelves) > 0 {
			shelves = append(shelves, finishedShelves...)
		}

		// 6. Fetch newest authors
		if lib.MediaType == "book" {
			naShelf, err := fetchNewestAuthorsShelf(db, libraryID, user, limitVal)
			if err == nil && naShelf != nil {
				shelves = append(shelves, *naShelf)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if shelves == nil {
			shelves = []Shelf{}
		}
		json.NewEncoder(w).Encode(shelves)
	}
}
