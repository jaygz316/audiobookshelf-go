package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/utils"
)

// handleAutoNumberSeries resolves POST /api/series/{id}/auto-number
func handleAutoNumberSeries(db *sql.DB, seriesID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST /api/series/%s/auto-number", seriesID)

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

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		type bookInfo struct {
			id            string
			publishedYear string
			publishedDate string
			title         string
		}

		rows, err := tx.Query(`
			SELECT b.id, b.publishedYear, b.publishedDate, b.title
			FROM bookSeries bs
			JOIN books b ON bs.bookId = b.id
			WHERE bs.seriesId = ?
		`, seriesID)
		if err != nil {
			http.Error(w, "failed to query books in series: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var books []bookInfo
		for rows.Next() {
			var b bookInfo
			var pubYear, pubDate sql.NullString
			if err := rows.Scan(&b.id, &pubYear, &pubDate, &b.title); err == nil {
				b.publishedYear = pubYear.String
				b.publishedDate = pubDate.String
				books = append(books, b)
			}
		}

		// Sort books chronologically:
		// 1. By publishedYear (if set)
		// 2. By publishedDate (if set)
		// 3. By title (alphabetically)
		sort.Slice(books, func(i, j int) bool {
			// Compare publishedYear
			yI := books[i].publishedYear
			yJ := books[j].publishedYear

			if yI != "" && yJ != "" {
				if yI != yJ {
					return yI < yJ
				}
			} else if yI != "" {
				return true
			} else if yJ != "" {
				return false
			}

			// Compare publishedDate
			dI := books[i].publishedDate
			dJ := books[j].publishedDate
			if dI != "" && dJ != "" {
				if dI != dJ {
					return dI < dJ
				}
			} else if dI != "" {
				return true
			} else if dJ != "" {
				return false
			}

			// Compare title
			return strings.ToLower(books[i].title) < strings.ToLower(books[j].title)
		})

		// Update sequences grouping by normalized title
		seqCounter := 0
		lastNormTitle := ""
		for _, b := range books {
			normTitle := utils.NormalizeTitleForSeries(b.title)
			if lastNormTitle == "" || normTitle != lastNormTitle {
				seqCounter++
				lastNormTitle = normTitle
			}
			seqStr := strconv.Itoa(seqCounter)
			_, err = tx.Exec(`
				UPDATE bookSeries
				SET sequence = ?
				WHERE bookId = ? AND seriesId = ?
			`, seqStr, b.id, seriesID)
			if err != nil {
				http.Error(w, "failed to update sequence: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}

		// Trigger websocket events for updated items
		for _, b := range books {
			var itemID string
			_ = tx.QueryRow("SELECT id FROM libraryItems WHERE mediaId = ? AND mediaType = 'book'", b.id).Scan(&itemID)
			if itemID != "" {
				if minItem, err := idb.GetLibraryItemMinifiedByID(db, itemID); err == nil {
					EmitLibraryItemEvent("item_updated", minItem)
				}
			}
		}

		err = tx.Commit()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true}`))
	}
}
