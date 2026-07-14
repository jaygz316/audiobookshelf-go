package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// NarratorJSON represents a narrator with a count of narrated books.
type NarratorJSON struct {
	Name     string `json:"name"`
	NumBooks int    `json:"numBooks"`
}

// handleGetLibraryNarrators resolves GET /api/libraries/{id}/narrators
func handleGetLibraryNarrators(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/libraries/%s/narrators", libraryID)

		sortBy := r.URL.Query().Get("sort")
		if sortBy == "" {
			sortBy = "name"
		}
		descVal := r.URL.Query().Get("desc")
		desc := descVal == "1" || descVal == "true"

		limitVal := r.URL.Query().Get("limit")
		limit, _ := strconv.Atoi(limitVal)
		pageVal := r.URL.Query().Get("page")
		page, _ := strconv.Atoi(pageVal)

		search := strings.ToLower(r.URL.Query().Get("search"))

		// Query all books in this library to get their narrators BLOB
		rows, err := db.Query(`
			SELECT b.narrators
			FROM books b
			JOIN libraryItems li ON li.mediaId = b.id AND li.mediaType = 'book'
			WHERE li.libraryId = ? AND li.isMissing = 0 AND li.isInvalid = 0
		`, libraryID)
		if err != nil {
			log.Printf("[Go] Failed to query narrators: %v", err)
			http.Error(w, `{"error": "Failed to query narrators"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		narratorCounts := make(map[string]int)

		for rows.Next() {
			var bNarrators []byte
			if err := rows.Scan(&bNarrators); err == nil && len(bNarrators) > 0 {
				var names []string
				if err := json.Unmarshal(bNarrators, &names); err == nil {
					for _, name := range names {
						name = strings.TrimSpace(name)
						if name != "" {
							narratorCounts[name]++
						}
					}
				}
			}
		}

		var results []NarratorJSON
		for name, count := range narratorCounts {
			// Apply search filter if specified
			if search != "" && !strings.Contains(strings.ToLower(name), search) {
				continue
			}
			results = append(results, NarratorJSON{
				Name:     name,
				NumBooks: count,
			})
		}

		// Sort
		sort.Slice(results, func(i, j int) bool {
			var less bool
			if sortBy == "numBooks" {
				less = results[i].NumBooks < results[j].NumBooks
				if results[i].NumBooks == results[j].NumBooks {
					less = strings.ToLower(results[i].Name) < strings.ToLower(results[j].Name)
				}
			} else {
				less = strings.ToLower(results[i].Name) < strings.ToLower(results[j].Name)
			}
			if desc {
				return !less
			}
			return less
		})

		total := len(results)
		paginatedResults := results
		if limit > 0 {
			startIndex := page * limit
			if startIndex > total {
				paginatedResults = []NarratorJSON{}
			} else {
				endIndex := startIndex + limit
				if endIndex > total {
					endIndex = total
				}
				paginatedResults = results[startIndex:endIndex]
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"results":   paginatedResults,
			"total":     total,
			"limit":     limit,
			"page":      page,
			"narrators": results,
		})
	}
}
