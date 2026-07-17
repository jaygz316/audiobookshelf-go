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

// handleGetLibraryAuthors resolves GET /api/libraries/{id}/authors
func handleGetLibraryAuthors(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/libraries/%s/authors", libraryID)

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

		limitVal := r.URL.Query().Get("limit")
		limit, _ := strconv.Atoi(limitVal)
		pageVal := r.URL.Query().Get("page")
		page, _ := strconv.Atoi(pageVal)

		search := strings.ToLower(r.URL.Query().Get("search"))

		// Query all authors for this library
		rows, err := db.Query(`
			SELECT id, name, lastFirst, asin, description, imagePath, createdAt, updatedAt
			FROM authors
			WHERE libraryId = ?
		`, libraryID)
		if err != nil {
			log.Errorf("[Go] Failed to query authors: %v", err)
			http.Error(w, `{"error": "Failed to query authors"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var authors []AuthorExpandedJSON
		for rows.Next() {
			var id, name string
			var lastFirst, asin, description, imagePath sql.NullString
			var createdAtStr, updatedAtStr string
			if err := rows.Scan(&id, &name, &lastFirst, &asin, &description, &imagePath, &createdAtStr, &updatedAtStr); err == nil {
				if search != "" && !strings.Contains(strings.ToLower(name), search) {
					continue
				}

				// Count distinct books associated with author in this library
				var numBooks int
				scanErr := db.QueryRow(`
					SELECT COUNT(DISTINCT ba.bookId)
					FROM bookAuthors ba
					JOIN libraryItems li ON li.mediaId = ba.bookId AND li.mediaType = 'book'
					WHERE ba.authorId = ? AND li.libraryId = ?
				`, id, libraryID).Scan(&numBooks)
				if scanErr != nil {
					log.Warnf("[Go Warning] Failed to count books for author %s: %v", id, scanErr)
				}

				authors = append(authors, AuthorExpandedJSON{
					ID:          id,
					Name:        name,
					LastFirst:   lastFirst.String,
					Asin:        asin.String,
					Description: description.String,
					ImagePath:   imagePath.String,
					AddedAt:     idb.ParseEpochMillis(createdAtStr),
					UpdatedAt:   idb.ParseEpochMillis(updatedAtStr),
					NumBooks:    numBooks,
				})
			} else {
				log.Warnf("[Go Warning] Failed to scan author: %v", err)
			}
		}

		// Memory sorting
		sort.Slice(authors, func(i, j int) bool {
			var less bool
			if sortBy == "numBooks" {
				less = authors[i].NumBooks < authors[j].NumBooks
				if authors[i].NumBooks == authors[j].NumBooks {
					less = strings.ToLower(authors[i].Name) < strings.ToLower(authors[j].Name)
				}
			} else {
				less = strings.ToLower(authors[i].Name) < strings.ToLower(authors[j].Name)
			}
			if desc {
				return !less
			}
			return less
		})

		// Paginate
		total := len(authors)
		results := authors
		if limit > 0 {
			startIndex := page * limit
			if startIndex > total {
				results = []AuthorExpandedJSON{}
			} else {
				endIndex := startIndex + limit
				if endIndex > total {
					endIndex = total
				}
				results = authors[startIndex:endIndex]
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"results": results,
			"total":   total,
			"limit":   limit,
			"page":    page,
			"authors": authors,
		})
	}
}
