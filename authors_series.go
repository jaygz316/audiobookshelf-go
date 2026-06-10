package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// AuthorExpandedJSON represents the expanded author object with book count
type AuthorExpandedJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	LastFirst   string `json:"lastFirst"`
	Asin        string `json:"asin"`
	Description string `json:"description"`
	ImagePath   string `json:"imagePath"`
	AddedAt     int64  `json:"addedAt"`
	UpdatedAt   int64  `json:"updatedAt"`
	NumBooks    int    `json:"numBooks"`
}

// GetAuthorImagePath retrieves the imagePath for a specific author
func GetAuthorImagePath(db *sql.DB, authorID string) (string, error) {
	var imagePath sql.NullString
	err := db.QueryRow("SELECT imagePath FROM authors WHERE id = ?", authorID).Scan(&imagePath)
	if err != nil {
		return "", err
	}
	if !imagePath.Valid {
		return "", nil
	}
	return imagePath.String, nil
}

// handleGetLibraryAuthors resolves GET /api/libraries/{id}/authors
func handleGetLibraryAuthors(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/libraries/%s/authors", libraryID)

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

		// Query all authors for this library
		rows, err := db.Query(`
			SELECT id, name, lastFirst, asin, description, imagePath, createdAt, updatedAt
			FROM authors
			WHERE libraryId = ?
		`, libraryID)
		if err != nil {
			log.Printf("[Go] Failed to query authors: %v", err)
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
				// Count distinct books associated with author in this library
				var numBooks int
				scanErr := db.QueryRow(`
					SELECT COUNT(DISTINCT ba.bookId)
					FROM bookAuthors ba
					JOIN libraryItems li ON li.mediaId = ba.bookId AND li.mediaType = 'book'
					WHERE ba.authorId = ? AND li.libraryId = ?
				`, id, libraryID).Scan(&numBooks)
				if scanErr != nil {
					log.Printf("[Go Warning] Failed to count books for author %s: %v", id, scanErr)
				}

				authors = append(authors, AuthorExpandedJSON{
					ID:          id,
					Name:        name,
					LastFirst:   lastFirst.String,
					Asin:        asin.String,
					Description: description.String,
					ImagePath:   imagePath.String,
					AddedAt:     parseEpochMillis(createdAtStr),
					UpdatedAt:   parseEpochMillis(updatedAtStr),
					NumBooks:    numBooks,
				})
			} else {
				log.Printf("[Go Warning] Failed to scan author: %v", err)
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
			"results":  results,
			"total":    total,
			"limit":    limit,
			"page":     page,
			"authors":  authors,
		})
	}
}

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
		log.Printf("[Go] GET /api/libraries/%s/series", libraryID)

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
			log.Printf("[Go] Failed to query series: %v", err)
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
							bAddedAt := parseEpochMillis(bCreatedAtStr)
							bUpdatedAt := parseEpochMillis(bUpdatedAtStr)
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
									"coverPath": nullIfEmpty(bCoverPath.String),
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
					AddedAt:              parseEpochMillis(createdAtStr),
					UpdatedAt:            parseEpochMillis(updatedAtStr),
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
		log.Printf("[Go] GET /api/libraries/%s/series/%s", libraryID, seriesID)

		userVal := r.Context().Value(UserContextKey)
		var userID string
		if userVal != nil {
			if u, ok := userVal.(*UserSession); ok {
				userID = u.ID
			}
		}

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
			"description":      nullIfEmpty(description.String),
			"addedAt":          parseEpochMillis(createdAtStr),
			"updatedAt":        parseEpochMillis(updatedAtStr),
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

// handleGetAuthorByID resolves GET /api/authors/{id}
func handleGetAuthorByID(db *sql.DB, authorID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/authors/%s", authorID)

		var id, name, lastFirst, createdAtStr, updatedAtStr string
		var asin, description, imagePath sql.NullString
		err := db.QueryRow(`
			SELECT id, name, lastFirst, asin, description, imagePath, createdAt, updatedAt
			FROM authors
			WHERE id = ?
		`, authorID).Scan(&id, &name, &lastFirst, &asin, &description, &imagePath, &createdAtStr, &updatedAtStr)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		includes := r.URL.Query().Get("include")
		includeItems := strings.Contains(includes, "items")
		includeSeries := strings.Contains(includes, "series")

		payload := map[string]interface{}{
			"id":          id,
			"name":        name,
			"lastFirst":   lastFirst,
			"asin":        nullIfEmpty(asin.String),
			"description": nullIfEmpty(description.String),
			"imagePath":   nullIfEmpty(imagePath.String),
			"addedAt":     parseEpochMillis(createdAtStr),
			"updatedAt":   parseEpochMillis(updatedAtStr),
		}

		if includeItems {
			// Query library items associated with author
			rows, err := db.Query(`
				SELECT li.id, li.ino, li.path, li.relPath, li.isFile, li.mtime, li.ctime, li.birthtime, li.createdAt, li.updatedAt, li.isMissing, li.isInvalid, li.mediaType, li.mediaId, li.size, li.libraryFolderId,
					b.title, b.titleIgnorePrefix, b.coverPath, b.tags, b.genres
				FROM libraryItems li
				JOIN bookAuthors ba ON ba.bookId = li.mediaId
				JOIN books b ON b.id = li.mediaId
				WHERE ba.authorId = ? AND li.mediaType = 'book'
			`, authorID)

			items := []interface{}{}
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var itemID, path, relPath, mediaType, mediaID, title string
					var ino, folderID, titleIgnorePrefix, coverPath sql.NullString
					var isFileVal, isMissingVal, isInvalidVal int
					var mtimeStr, ctimeStr, birthtimeStr, createdAtStr, updatedAtStr string
					var size int64
					var tagsBytes, genresBytes []byte

					err := rows.Scan(
						&itemID, &ino, &path, &relPath, &isFileVal, &mtimeStr, &ctimeStr, &birthtimeStr, &createdAtStr, &updatedAtStr, &isMissingVal, &isInvalidVal, &mediaType, &mediaID, &size, &folderID,
						&title, &titleIgnorePrefix, &coverPath, &tagsBytes, &genresBytes,
					)
					if err == nil {
						var tags []string
						_ = json.Unmarshal(tagsBytes, &tags)
						var genres []string
						_ = json.Unmarshal(genresBytes, &genres)

						items = append(items, map[string]interface{}{
							"id":               itemID,
							"ino":              ino.String,
							"path":             path,
							"relPath":          relPath,
							"isFile":           isFileVal != 0,
							"mtimeMs":          parseEpochMillis(mtimeStr),
							"ctimeMs":          parseEpochMillis(ctimeStr),
							"birthtimeMs":      parseEpochMillis(birthtimeStr),
							"addedAt":          parseEpochMillis(createdAtStr),
							"updatedAt":        parseEpochMillis(updatedAtStr),
							"isMissing":        isMissingVal != 0,
							"isInvalid":        isInvalidVal != 0,
							"mediaType":        mediaType,
							"size":             size,
							"libraryFolderId":  folderID.String,
							"media": map[string]interface{}{
								"id":        mediaID,
								"coverPath": nullIfEmpty(coverPath.String),
								"tags":      tags,
								"metadata": map[string]interface{}{
									"title":             title,
									"titleIgnorePrefix": titleIgnorePrefix.String,
									"genres":            genres,
								},
							},
						})
					} else {
						log.Printf("[Go Warning] Failed to scan author item: %v", err)
					}
				}
			}
			payload["libraryItems"] = items
		}

		if includeSeries {
			// Query distinct series for this author
			rows, err := db.Query(`
				SELECT DISTINCT s.id, s.name
				FROM series s
				JOIN bookSeries bs ON bs.seriesId = s.id
				JOIN bookAuthors ba ON ba.bookId = bs.bookId
				WHERE ba.authorId = ?
			`, authorID)

			var series []interface{}
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var sID, sName string
					if err := rows.Scan(&sID, &sName); err == nil {
						// Query books inside this series by this author
						bookRows, err := db.Query(`
							SELECT li.id, li.ino, li.path, li.relPath, li.isFile, li.mtime, li.ctime, li.birthtime, li.createdAt, li.updatedAt, li.isMissing, li.isInvalid, li.mediaType, li.mediaId, li.size, li.libraryFolderId,
								b.title, b.titleIgnorePrefix, b.coverPath, b.tags, b.genres, bs.sequence
							FROM libraryItems li
							JOIN bookAuthors ba ON ba.bookId = li.mediaId
							JOIN bookSeries bs ON bs.bookId = li.mediaId
							JOIN books b ON b.id = li.mediaId
							WHERE ba.authorId = ? AND bs.seriesId = ? AND li.mediaType = 'book'
						`, authorID, sID)

						books := []interface{}{}
						if err == nil {
							for bookRows.Next() {
								var itemID, path, relPath, mediaType, mediaID, title string
								var ino, folderID, titleIgnorePrefix, coverPath, sequence sql.NullString
								var isFileVal, isMissingVal, isInvalidVal int
								var mtimeStr, ctimeStr, birthtimeStr, createdAtStr, updatedAtStr string
								var size int64
								var tagsBytes, genresBytes []byte

								err := bookRows.Scan(
									&itemID, &ino, &path, &relPath, &isFileVal, &mtimeStr, &ctimeStr, &birthtimeStr, &createdAtStr, &updatedAtStr, &isMissingVal, &isInvalidVal, &mediaType, &mediaID, &size, &folderID,
									&title, &titleIgnorePrefix, &coverPath, &tagsBytes, &genresBytes, &sequence,
								)
								if err == nil {
									var tags []string
									_ = json.Unmarshal(tagsBytes, &tags)
									var genres []string
									_ = json.Unmarshal(genresBytes, &genres)

									books = append(books, map[string]interface{}{
										"id":               itemID,
										"ino":              ino.String,
										"path":             path,
										"relPath":          relPath,
										"isFile":           isFileVal != 0,
										"mtimeMs":          parseEpochMillis(mtimeStr),
										"ctimeMs":          parseEpochMillis(ctimeStr),
										"birthtimeMs":      parseEpochMillis(birthtimeStr),
										"addedAt":          parseEpochMillis(createdAtStr),
										"updatedAt":        parseEpochMillis(updatedAtStr),
										"isMissing":        isMissingVal != 0,
										"isInvalid":        isInvalidVal != 0,
										"mediaType":        mediaType,
										"size":             size,
										"libraryFolderId":  folderID.String,
										"sequence":         sequence.String,
										"media": map[string]interface{}{
											"id":        mediaID,
											"coverPath": nullIfEmpty(coverPath.String),
											"tags":      tags,
											"metadata": map[string]interface{}{
												"title":             title,
												"titleIgnorePrefix": titleIgnorePrefix.String,
												"genres":            genres,
											},
										},
									})
								} else {
									log.Printf("[Go Warning] Failed to scan author series book: %v", err)
								}
							}
							bookRows.Close()
						}

						series = append(series, map[string]interface{}{
							"id":    sID,
							"name":  sName,
							"items": books,
						})
					}
				}
			}
			payload["series"] = series
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

// handleGetAuthorImage serves the author image file
func handleGetAuthorImage(db *sql.DB, metadataPath string, authorID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/authors/%s/image", authorID)

		imagePath, err := GetAuthorImagePath(db, authorID)
		if err != nil || imagePath == "" {
			http.NotFound(w, r)
			return
		}

		var fullPath string
		if filepath.IsAbs(imagePath) {
			fullPath = imagePath
		} else {
			fullPath = filepath.Join(metadataPath, imagePath)
		}

		if _, err := os.Stat(fullPath); err != nil {
			log.Printf("[Go] Author image not found: %s", fullPath)
			http.NotFound(w, r)
			return
		}

		if r.URL.Query().Get("ts") != "" {
			w.Header().Set("Cache-Control", "private, max-age=86400")
		}
		http.ServeFile(w, r, fullPath)
	}
}

// handleMatchAuthor stubs out author matching
func handleMatchAuthor(db *sql.DB, authorID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /api/authors/%s/match (stub)", authorID)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"updated": false}`))
	}
}

// handleGetLibraryItemByID resolves GET /api/items/{id}
func handleGetLibraryItemByID(db *sql.DB, itemID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/items/%s", itemID)

		var id, ino, libraryID, folderID, path, relPath, mediaType, mediaID, mtimeStr, ctimeStr, birthtimeStr, createdAtStr, updatedAtStr string
		var isFileVal, isMissingVal, isInvalidVal int
		var size int64

		query := `
			SELECT id, ino, libraryId, libraryFolderId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size
			FROM libraryItems
			WHERE id = ?
		`
		err := db.QueryRow(query, itemID).Scan(
			&id, &ino, &libraryID, &folderID, &path, &relPath, &isFileVal, &mtimeStr, &ctimeStr, &birthtimeStr, &createdAtStr, &updatedAtStr, &isMissingVal, &isInvalidVal, &mediaType, &mediaID, &size,
		)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		payload := map[string]interface{}{
			"id":              id,
			"ino":             ino,
			"libraryId":       libraryID,
			"folderId":        folderID,
			"path":            path,
			"relPath":         relPath,
			"isFile":          isFileVal != 0,
			"mtimeMs":         parseEpochMillis(mtimeStr),
			"ctimeMs":         parseEpochMillis(ctimeStr),
			"birthtimeMs":     parseEpochMillis(birthtimeStr),
			"addedAt":         parseEpochMillis(createdAtStr),
			"updatedAt":       parseEpochMillis(updatedAtStr),
			"isMissing":       isMissingVal != 0,
			"isInvalid":       isInvalidVal != 0,
			"mediaType":       mediaType,
			"size":            size,
			"libraryFiles":    []interface{}{},
		}

		if mediaType == "book" {
			var bTitle string
			var bTitleIgnorePrefix, bSubtitle, bPublishedYear, bPublishedDate, bPublisher, bDescription, bIsbn, bAsin, bLanguage, bCoverPath sql.NullString
			var bDuration float64
			var bNarrators, bAudioFiles, bEbookFile, bChapters, bTags, bGenres []byte
			var bExplicit, bAbridged sql.NullInt64

			err = db.QueryRow(`
				SELECT title, titleIgnorePrefix, subtitle, publishedYear, publishedDate, publisher, description, isbn, asin, language, explicit, abridged, coverPath, duration, narrators, audioFiles, ebookFile, chapters, tags, genres
				FROM books WHERE id = ?
			`, mediaID).Scan(
				&bTitle, &bTitleIgnorePrefix, &bSubtitle, &bPublishedYear, &bPublishedDate, &bPublisher, &bDescription, &bIsbn, &bAsin, &bLanguage, &bExplicit, &bAbridged, &bCoverPath, &bDuration, &bNarrators, &bAudioFiles, &bEbookFile, &bChapters, &bTags, &bGenres,
			)
			if err == nil {
				var tags []string
				_ = json.Unmarshal(bTags, &tags)
				var genres []string
				_ = json.Unmarshal(bGenres, &genres)
				var audioFiles []map[string]interface{}
				_ = json.Unmarshal(bAudioFiles, &audioFiles)
				var ebook interface{}
				_ = json.Unmarshal(bEbookFile, &ebook)
				var chapters []interface{}
				_ = json.Unmarshal(bChapters, &chapters)

				var authorNames []string
				var seriesNames []string
				var narratorNames []string
				_ = json.Unmarshal(bNarrators, &narratorNames)

				// Query authors
				rows, err := db.Query("SELECT name FROM authors WHERE id IN (SELECT authorId FROM bookAuthors WHERE bookId = ?)", mediaID)
				if err == nil {
					defer rows.Close()
					for rows.Next() {
						var name string
						if err := rows.Scan(&name); err == nil {
							authorNames = append(authorNames, name)
						}
					}
				}

				// Query series
				srows, err := db.Query("SELECT name FROM series WHERE id IN (SELECT seriesId FROM bookSeries WHERE bookId = ?)", mediaID)
				if err == nil {
					defer srows.Close()
					for srows.Next() {
						var name string
						if err := srows.Scan(&name); err == nil {
							seriesNames = append(seriesNames, name)
						}
					}
				}

				authorName := strings.Join(authorNames, ", ")
				seriesName := strings.Join(seriesNames, ", ")
				narratorName := strings.Join(narratorNames, ", ")

				var ebookFormat *string
				if len(bEbookFile) > 0 {
					var eb struct {
						EbookFormat string `json:"ebookFormat"`
					}
					if json.Unmarshal(bEbookFile, &eb) == nil && eb.EbookFormat != "" {
						ebookFormat = &eb.EbookFormat
					}
				}

				payload["media"] = map[string]interface{}{
					"id":            mediaID,
					"coverPath":     nullIfEmpty(bCoverPath.String),
					"tags":          tags,
					"numTracks":     len(audioFiles),
					"numAudioFiles": len(audioFiles),
					"numChapters":   len(chapters),
					"duration":      bDuration,
					"size":          size,
					"ebookFormat":   ebookFormat,
					"audioFiles":    audioFiles,
					"ebookFile":     ebook,
					"chapters":      chapters,
					"metadata": map[string]interface{}{
						"title":             bTitle,
						"titleIgnorePrefix": bTitleIgnorePrefix.String,
						"subtitle":          nullIfEmpty(bSubtitle.String),
						"authorName":        authorName,
						"authorNameLF":      nameToLastFirst(authorName),
						"narratorName":      narratorName,
						"seriesName":        seriesName,
						"genres":            genres,
						"publishedYear":     nullIfEmpty(bPublishedYear.String),
						"publishedDate":     nullIfEmpty(bPublishedDate.String),
						"publisher":         nullIfEmpty(bPublisher.String),
						"description":       nullIfEmpty(bDescription.String),
						"isbn":              nullIfEmpty(bIsbn.String),
						"asin":              nullIfEmpty(bAsin.String),
						"language":          nullIfEmpty(bLanguage.String),
						"explicit":          bExplicit.Valid && bExplicit.Int64 != 0,
						"abridged":          bAbridged.Valid && bAbridged.Int64 != 0,
					},
				}

				// Dynamically construct libraryFiles from ebookFile and audioFiles
				var libraryFiles []interface{}

				// Parse ebookFile metadata
				if len(bEbookFile) > 0 {
					var eb struct {
						Metadata struct {
							Filename string `json:"filename"`
							Ext      string `json:"ext"`
							Path     string `json:"path"`
							RelPath  string `json:"relPath"`
							Size     int64  `json:"size"`
							Ctime    int64  `json:"ctime"`
							Mtime    int64  `json:"mtime"`
						} `json:"metadata"`
					}
					if json.Unmarshal(bEbookFile, &eb) == nil && eb.Metadata.Filename != "" {
						libraryFiles = append(libraryFiles, map[string]interface{}{
							"ino":      ino, // Use the item's inode
							"filename": eb.Metadata.Filename,
							"ext":      eb.Metadata.Ext,
							"path":     eb.Metadata.Path,
							"relPath":  eb.Metadata.RelPath,
							"size":     eb.Metadata.Size,
							"fileType": "ebook",
							"mtime":    eb.Metadata.Mtime,
							"ctime":    eb.Metadata.Ctime,
						})
					}
				}

				// Map audioFiles to libraryFiles
				for _, af := range audioFiles {
					lfItem := map[string]interface{}{
						"fileType": "audio",
					}
					if val, ok := af["ino"]; ok {
						lfItem["ino"] = val
					}
					if val, ok := af["filename"]; ok {
						lfItem["filename"] = val
					}
					if val, ok := af["ext"]; ok {
						lfItem["ext"] = val
					}
					if val, ok := af["size"]; ok {
						lfItem["size"] = val
					}
					if metadata, ok := af["metadata"].(map[string]interface{}); ok {
						if val, ok := metadata["path"]; ok {
							lfItem["path"] = val
						}
						if val, ok := metadata["relPath"]; ok {
							lfItem["relPath"] = val
						}
						if val, ok := metadata["mtime"]; ok {
							lfItem["mtime"] = val
						}
						if val, ok := metadata["ctime"]; ok {
							lfItem["ctime"] = val
						}
					}
					libraryFiles = append(libraryFiles, lfItem)
				}

				payload["libraryFiles"] = libraryFiles
			} else {
				log.Printf("[Go Warning] Failed to scan book with id %s: %v", mediaID, err)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

// handleServeEbook serves the EPUB/PDF ebook file
func handleServeEbook(db *sql.DB, itemID string, fileID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/items/%s/ebook (fileID=%s)", itemID, fileID)

		var mediaID, mediaType string
		err := db.QueryRow("SELECT mediaId, mediaType FROM libraryItems WHERE id = ?", itemID).Scan(&mediaID, &mediaType)
		if err != nil || mediaType != "book" {
			http.NotFound(w, r)
			return
		}

		var ebookFileBytes []byte
		err = db.QueryRow("SELECT ebookFile FROM books WHERE id = ?", mediaID).Scan(&ebookFileBytes)
		if err != nil || len(ebookFileBytes) == 0 {
			http.NotFound(w, r)
			return
		}

		var ebook struct {
			EbookFormat string `json:"ebookFormat"`
			Metadata    struct {
				Path string `json:"path"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(ebookFileBytes, &ebook); err != nil {
			http.Error(w, "invalid ebook metadata", http.StatusInternalServerError)
			return
		}

		filePath := ebook.Metadata.Path
		if _, err := os.Stat(filePath); err != nil {
			log.Printf("[Go] Ebook file not found: %s", filePath)
			http.NotFound(w, r)
			return
		}

		ext := strings.ToLower(filepath.Ext(filePath))
		switch ext {
		case ".epub":
			w.Header().Set("Content-Type", "application/epub+zip")
		case ".pdf":
			w.Header().Set("Content-Type", "application/pdf")
		case ".mobi":
			w.Header().Set("Content-Type", "application/x-mobipocket-ebook")
		case ".cbz":
			w.Header().Set("Content-Type", "application/x-cbz")
		case ".cbr":
			w.Header().Set("Content-Type", "application/x-cbr")
		}

		http.ServeFile(w, r, filePath)
	}
}
