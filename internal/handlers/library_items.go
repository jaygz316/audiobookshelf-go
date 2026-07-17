package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

func HandleGetLibraryItems(db *sql.DB, libraryID string) http.HandlerFunc {
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
		limitVal := 0
		if q.Get("limit") != "" {
			fmt.Sscanf(q.Get("limit"), "%d", &limitVal)
		}
		pageVal := 0
		if q.Get("page") != "" {
			fmt.Sscanf(q.Get("page"), "%d", &pageVal)
		}

		sortBy := q.Get("sort")
		sortDesc := q.Get("desc") == "1"
		filterBy := q.Get("filter")
		minified := q.Get("minified") == "1"
		collapseseries := q.Get("collapseseries") == "1"
		include := q.Get("include")
		searchQuery := q.Get("search")

		var includeArray []string
		if include != "" {
			for _, part := range strings.Split(include, ",") {
				includeArray = append(includeArray, strings.TrimSpace(part))
			}
		}

		opts := idb.GetFilteredLibraryItemsOptions{
			LibraryID:      libraryID,
			User:           user,
			FilterBy:       filterBy,
			SortBy:         sortBy,
			SortDesc:       sortDesc,
			Limit:          limitVal,
			Page:           pageVal,
			CollapseSeries: collapseseries,
			Include:        includeArray,
			MediaType:      lib.MediaType,
			Minified:       minified,
			Search:         searchQuery,
		}

		results, total, err := idb.GetFilteredLibraryItems(db, opts)
		if err != nil {
			log.Errorf("[Go] Failed to get filtered items for library %s: %v", libraryID, err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		payload := map[string]interface{}{
			"results":        results,
			"total":          total,
			"limit":          limitVal,
			"page":           pageVal,
			"sortBy":         sortBy,
			"sortDesc":       sortDesc,
			"filterBy":       filterBy,
			"mediaType":      lib.MediaType,
			"minified":       minified,
			"collapseseries": collapseseries,
			"include":        include,
			"search":         searchQuery,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}
