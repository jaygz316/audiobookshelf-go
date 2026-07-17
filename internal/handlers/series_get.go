package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

// handleGetLibrarySeries resolves GET /api/libraries/{id}/series
func handleGetLibrarySeries(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/libraries/%s/series", libraryID)

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

		sortBy := r.URL.Query().Get("sort")
		if sortBy == "" {
			sortBy = "name"
		}
		descVal := r.URL.Query().Get("desc")
		desc := descVal == "1" || descVal == "true"

		filter := r.URL.Query().Get("filter")

		limitVal := r.URL.Query().Get("limit")
		limit, _ := strconv.Atoi(limitVal)
		pageVal := r.URL.Query().Get("page")
		page, _ := strconv.Atoi(pageVal)

		// Fetch all series for this library
		rows, err := db.Query(`
			SELECT id, name, nameIgnorePrefix, description, createdAt, updatedAt
			FROM series
			WHERE libraryId = ?
		`, libraryID)
		if err != nil {
			log.Errorf("[Go] Failed to query series: %v", err)
			http.Error(w, `{"error": "Failed to query series"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var list []SeriesBooksJSON
		for rows.Next() {
			var id, name string
			var nameIgnorePrefix, description sql.NullString
			var createdAtStr, updatedAtStr string
			if err := rows.Scan(&id, &name, &nameIgnorePrefix, &description, &createdAtStr, &updatedAtStr); err == nil {
				// Query books inside this series and user's progress using the extracted helper
				books, totalDuration, lastBookAdded, lastBookUpdated, libraryItemIds, libraryItemIdsFinished, err := fetchSeriesBooksMinified(db, user.ID, id, libraryID)
				if err != nil {
					log.Errorf("Failed to fetch books for series %s: %v", id, err)
					continue
				}

				var seriesProgress *SeriesProgress
				if len(libraryItemIds) > 0 {
					seriesProgress = &SeriesProgress{
						LibraryItemIds:         libraryItemIds,
						LibraryItemIdsFinished: libraryItemIdsFinished,
						IsFinished:             len(libraryItemIdsFinished) == len(libraryItemIds),
					}
				}

				list = append(list, SeriesBooksJSON{
					ID:                   id,
					Name:                 name,
					AddedAt:              idb.ParseEpochMillis(createdAtStr),
					UpdatedAt:            idb.ParseEpochMillis(updatedAtStr),
					NameIgnorePrefix:     nameIgnorePrefix.String,
					NameIgnorePrefixSort: strings.TrimSpace(nameIgnorePrefix.String),
					Type:                 "series",
					Books:                books,
					TotalDuration:        totalDuration,
					Progress:             seriesProgress,
					LastBookAdded:        lastBookAdded,
					LastBookUpdated:      lastBookUpdated,
				})
			}
		}

		// Apply search filter
		if filter != "" && filter != "all" {
			var filtered []SeriesBooksJSON
			filterLower := strings.ToLower(filter)
			for _, s := range list {
				if strings.Contains(strings.ToLower(s.Name), filterLower) {
					filtered = append(filtered, s)
				}
			}
			list = filtered
		}

		// Memory sorting
		sort.Slice(list, func(i, j int) bool {
			var less bool
			switch sortBy {
			case "numBooks":
				less = len(list[i].Books) < len(list[j].Books)
				if len(list[i].Books) == len(list[j].Books) {
					less = strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
				}
			case "totalDuration":
				less = list[i].TotalDuration < list[j].TotalDuration
				if list[i].TotalDuration == list[j].TotalDuration {
					less = strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
				}
			case "addedAt":
				less = list[i].AddedAt < list[j].AddedAt
			case "lastBookAdded":
				less = list[i].LastBookAdded < list[j].LastBookAdded
			case "lastBookUpdated":
				less = list[i].LastBookUpdated < list[j].LastBookUpdated
			default: // name
				less = strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
			}
			if desc {
				return !less
			}
			return less
		})

		// Paginate
		total := len(list)
		results := list
		if limit > 0 {
			startIndex := page * limit
			if startIndex > total {
				results = []SeriesBooksJSON{}
			} else {
				endIndex := startIndex + limit
				if endIndex > total {
					endIndex = total
				}
				results = list[startIndex:endIndex]
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"results":  results,
			"total":    total,
			"limit":    limit,
			"page":     page,
			"sortBy":   sortBy,
			"sortDesc": desc,
			"filterBy": filter,
		})
	}
}
