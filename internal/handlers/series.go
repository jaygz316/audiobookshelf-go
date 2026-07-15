package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	isocket "audiobookshelf/internal/socket"
	"audiobookshelf/internal/utils"
)

// BookSequenceMinified represents a book inside the series with sequence
type BookSequenceMinified struct {
	ID        string      `json:"id"`
	MediaType string      `json:"mediaType"`
	UpdatedAt int64       `json:"updatedAt"`
	AddedAt   int64       `json:"addedAt"`
	Sequence  string      `json:"sequence"`
	Media     interface{} `json:"media"`
}

// SeriesBooksJSON represents the series details with books in it
type SeriesBooksJSON struct {
	ID                   string                 `json:"id"`
	Name                 string                 `json:"name"`
	AddedAt              int64                  `json:"addedAt"`
	UpdatedAt            int64                  `json:"updatedAt"`
	NameIgnorePrefix     string                 `json:"nameIgnorePrefix"`
	NameIgnorePrefixSort string                 `json:"nameIgnorePrefixSort"`
	Type                 string                 `json:"type"`
	Books                []BookSequenceMinified `json:"books"`
	TotalDuration        float64                `json:"totalDuration"`
	LastBookAdded        int64                  `json:"-"`
	LastBookUpdated      int64                  `json:"-"`
}

func parseSequence(s string) float64 {
	if s == "" {
		return 9999.0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 9999.0
	}
	return f
}

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
				// Query books inside this series
				bookRows, err := db.Query(`
					SELECT li.id, b.coverPath, bs.sequence, li.updatedAt, li.createdAt, b.duration, b.title, b.titleIgnorePrefix
					FROM bookSeries bs
					JOIN libraryItems li ON li.mediaId = bs.bookId AND li.mediaType = 'book'
					JOIN books b ON b.id = li.mediaId
					WHERE bs.seriesId = ? AND li.libraryId = ?
				`, id, libraryID)

				books := []BookSequenceMinified{}
				var totalDuration float64
				var lastBookAdded int64
				var lastBookUpdated int64

				if err == nil {
					for bookRows.Next() {
						var bLID, bUpdatedAtStr, bCreatedAtStr, bTitle string
						var bCoverPath, bSequence, bTitleIgnorePrefix sql.NullString
						var bDuration float64
						if err := bookRows.Scan(&bLID, &bCoverPath, &bSequence, &bUpdatedAtStr, &bCreatedAtStr, &bDuration, &bTitle, &bTitleIgnorePrefix); err == nil {
							bAddedAt := idb.ParseEpochMillis(bCreatedAtStr)
							bUpdatedAt := idb.ParseEpochMillis(bUpdatedAtStr)
							totalDuration += bDuration
							if bAddedAt > lastBookAdded {
								lastBookAdded = bAddedAt
							}
							if bUpdatedAt > lastBookUpdated {
								lastBookUpdated = bUpdatedAt
							}

							books = append(books, BookSequenceMinified{
								ID:        bLID,
								MediaType: "book",
								UpdatedAt: bUpdatedAt,
								AddedAt:   bAddedAt,
								Sequence:  bSequence.String,
								Media: map[string]interface{}{
									"coverPath": utils.NullIfEmpty(bCoverPath.String),
									"metadata": map[string]interface{}{
										"title":             bTitle,
										"titleIgnorePrefix": bTitleIgnorePrefix.String,
									},
								},
							})
						}
					}
					bookRows.Close()
				}

				// Sort books by sequence asc
				sort.Slice(books, func(i, j int) bool {
					return parseSequence(books[i].Sequence) < parseSequence(books[j].Sequence)
				})

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

// handleGetLibrarySeriesByID resolves GET /api/libraries/{id}/series/{seriesId}
func handleGetLibrarySeriesByID(db *sql.DB, libraryID string, seriesID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/libraries/%s/series/%s", libraryID, seriesID)

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
		userID := user.ID

		// Retrieve series metadata
		var id, name string
		var nameIgnorePrefix, description sql.NullString
		var createdAtStr, updatedAtStr string
		err := db.QueryRow(`
			SELECT id, name, nameIgnorePrefix, description, createdAt, updatedAt
			FROM series
			WHERE id = ? AND libraryId = ?
		`, seriesID, libraryID).Scan(&id, &name, &nameIgnorePrefix, &description, &createdAtStr, &updatedAtStr)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		// Query the user's progress for all books in the series
		rows, err := db.Query(`
			SELECT li.id, mp.isFinished
			FROM bookSeries bs
			JOIN libraryItems li ON li.mediaId = bs.bookId AND li.mediaType = 'book'
			LEFT JOIN mediaProgresses mp ON mp.mediaItemId = bs.bookId AND mp.userId = ?
			WHERE bs.seriesId = ? AND li.libraryId = ?
		`, userID, seriesID, libraryID)

		libraryItemIds := []string{}
		libraryItemIdsFinished := []string{}

		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var itemID string
				var isFinished sql.NullInt64
				if err := rows.Scan(&itemID, &isFinished); err == nil {
					libraryItemIds = append(libraryItemIds, itemID)
					if isFinished.Valid && isFinished.Int64 == 1 {
						libraryItemIdsFinished = append(libraryItemIdsFinished, itemID)
					}
				}
			}
		}

		isFinished := len(libraryItemIdsFinished) == len(libraryItemIds) && len(libraryItemIds) > 0

		payload := map[string]interface{}{
			"id":               id,
			"name":             name,
			"nameIgnorePrefix": nameIgnorePrefix.String,
			"description":      utils.NullIfEmpty(description.String),
			"addedAt":          idb.ParseEpochMillis(createdAtStr),
			"updatedAt":        idb.ParseEpochMillis(updatedAtStr),
			"progress": map[string]interface{}{
				"libraryItemIds":         libraryItemIds,
				"libraryItemIdsFinished": libraryItemIdsFinished,
				"isFinished":             isFinished,
			},
			"rssFeed": nil,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

// handleUpdateSeries resolves PATCH /api/series/{id}
func handleUpdateSeries(db *sql.DB, seriesID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] PATCH /api/series/%s", seriesID)

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
			Name             string `json:"name"`
			NameIgnorePrefix string `json:"nameIgnorePrefix"`
			Description      string `json:"description"`
		}

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

		_, err = tx.Exec(`
			UPDATE series
			SET name = ?, nameIgnorePrefix = ?, description = ?, updatedAt = ?
			WHERE id = ?
		`, payload.Name, payload.NameIgnorePrefix, payload.Description, nowStr, seriesID)
		if err != nil {
			http.Error(w, "failed to update series: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Update linked library items sidecar metadata if needed
		rows, err := tx.Query("SELECT bookId FROM bookSeries WHERE seriesId = ?", seriesID)
		if err == nil {
			var bookIDs []string
			for rows.Next() {
				var bid string
				if err := rows.Scan(&bid); err == nil {
					bookIDs = append(bookIDs, bid)
				}
			}
			rows.Close()

			for _, bid := range bookIDs {
				// Emit real-time update
				if isocket.GlobalAuth != nil {
					var itemID string
					_ = tx.QueryRow("SELECT id FROM libraryItems WHERE mediaId = ? AND mediaType = 'book'", bid).Scan(&itemID)
					if itemID != "" {
						if minItem, err := idb.GetLibraryItemMinifiedByID(db, itemID); err == nil {
							EmitLibraryItemEvent("item_updated", minItem)
						}
					}
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
