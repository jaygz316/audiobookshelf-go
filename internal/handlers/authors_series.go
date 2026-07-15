package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/providers"
	iscanner "audiobookshelf/internal/scanner"
	isocket "audiobookshelf/internal/socket"
	"audiobookshelf/internal/utils"
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

// handleGetAuthorByID resolves GET /api/authors/{id}
func handleGetAuthorByID(db *sql.DB, authorID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/authors/%s", authorID)

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
			"asin":        utils.NullIfEmpty(asin.String),
			"description": utils.NullIfEmpty(description.String),
			"imagePath":   utils.NullIfEmpty(imagePath.String),
			"addedAt":     idb.ParseEpochMillis(createdAtStr),
			"updatedAt":   idb.ParseEpochMillis(updatedAtStr),
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
							"id":              itemID,
							"ino":             ino.String,
							"path":            path,
							"relPath":         relPath,
							"isFile":          isFileVal != 0,
							"mtimeMs":         idb.ParseEpochMillis(mtimeStr),
							"ctimeMs":         idb.ParseEpochMillis(ctimeStr),
							"birthtimeMs":     idb.ParseEpochMillis(birthtimeStr),
							"addedAt":         idb.ParseEpochMillis(createdAtStr),
							"updatedAt":       idb.ParseEpochMillis(updatedAtStr),
							"isMissing":       isMissingVal != 0,
							"isInvalid":       isInvalidVal != 0,
							"mediaType":       mediaType,
							"size":            size,
							"libraryFolderId": folderID.String,
							"media": map[string]interface{}{
								"id":        mediaID,
								"coverPath": utils.NullIfEmpty(coverPath.String),
								"tags":      tags,
								"metadata": map[string]interface{}{
									"title":             title,
									"titleIgnorePrefix": titleIgnorePrefix.String,
									"genres":            genres,
								},
							},
						})
					} else {
						log.Warnf("[Go Warning] Failed to scan author item: %v", err)
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
										"id":              itemID,
										"ino":             ino.String,
										"path":            path,
										"relPath":         relPath,
										"isFile":          isFileVal != 0,
										"mtimeMs":         idb.ParseEpochMillis(mtimeStr),
										"ctimeMs":         idb.ParseEpochMillis(ctimeStr),
										"birthtimeMs":     idb.ParseEpochMillis(birthtimeStr),
										"addedAt":         idb.ParseEpochMillis(createdAtStr),
										"updatedAt":       idb.ParseEpochMillis(updatedAtStr),
										"isMissing":       isMissingVal != 0,
										"isInvalid":       isInvalidVal != 0,
										"mediaType":       mediaType,
										"size":            size,
										"libraryFolderId": folderID.String,
										"sequence":        sequence.String,
										"media": map[string]interface{}{
											"id":        mediaID,
											"coverPath": utils.NullIfEmpty(coverPath.String),
											"tags":      tags,
											"metadata": map[string]interface{}{
												"title":             title,
												"titleIgnorePrefix": titleIgnorePrefix.String,
												"genres":            genres,
											},
										},
									})
								} else {
									log.Warnf("[Go Warning] Failed to scan author series book: %v", err)
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
		log.Infof("[Go] GET /api/authors/%s/image", authorID)

		imagePath, err := idb.GetAuthorImagePath(db, authorID)
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
			log.Warnf("[Go] Author image not found: %s", fullPath)
			http.NotFound(w, r)
			return
		}

		if !utils.IsSafeFilePath(db, metadataPath, fullPath) {
			log.Warnf("[Go] Author image path traversal blocked: %s", fullPath)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if r.URL.Query().Get("ts") != "" {
			w.Header().Set("Cache-Control", "private, max-age=86400")
		}
		http.ServeFile(w, r, fullPath)
	}
}

// handleDeleteAuthorImage handles DELETE /api/authors/{id}/image
func handleDeleteAuthorImage(db *sql.DB, cfg *core.Config, authorID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] DELETE /api/authors/%s/image", authorID)

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

		imagePath, err := idb.GetAuthorImagePath(db, authorID)
		if err != nil {
			http.Error(w, "failed to check author image: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if imagePath != "" {
			var fullPath string
			if filepath.IsAbs(imagePath) {
				fullPath = imagePath
			} else {
				fullPath = filepath.Join(cfg.MetadataPath, imagePath)
			}
			if utils.IsSafeFilePath(db, cfg.MetadataPath, fullPath) {
				_ = os.Remove(fullPath)
			} else {
				log.Warnf("[DeleteAuthorImage] Blocked deletion of unsafe author image path: %s", fullPath)
			}
		}

		nowStr := time.Now().Format("2006-01-02 15:04:05.000")
		_, err = db.Exec("UPDATE authors SET imagePath = '', updatedAt = ? WHERE id = ?", nowStr, authorID)
		if err != nil {
			http.Error(w, "failed to update database: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"updated": true}`))
	}
}

// handleMatchAuthor handles POST /api/authors/{id}/match
func handleMatchAuthor(db *sql.DB, cfg *core.Config, authorID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST /api/authors/%s/match", authorID)

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
			ASIN     string `json:"asin"`
			Provider string `json:"provider"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		if payload.ASIN == "" {
			http.Error(w, `{"error": "asin parameter is required"}`, http.StatusBadRequest)
			return
		}

		initManagers(db)
		prov, ok := globalFinder.Providers()["audnexus"]
		if !ok {
			http.Error(w, `{"error": "audnexus provider not registered"}`, http.StatusInternalServerError)
			return
		}
		audnexus, ok := prov.(*providers.AudnexusProvider)
		if !ok {
			http.Error(w, `{"error": "failed to cast to AudnexusProvider"}`, http.StatusInternalServerError)
			return
		}

		details, err := audnexus.AuthorRequest(r.Context(), payload.ASIN, "")
		if err != nil {
			log.Errorf("[MatchAuthor] AuthorRequest failed: %v", err)
			http.Error(w, fmt.Sprintf("Failed to fetch author details from provider: %v", err), http.StatusInternalServerError)
			return
		}
		if details == nil {
			http.Error(w, `{"error": "Author details not found on provider"}`, http.StatusNotFound)
			return
		}

		// Download author image if present
		localImagePath := ""
		if details.Image != "" {
			// Ensure metadata/authors folder exists
			authorsDir := filepath.Join(cfg.MetadataPath, "authors")
			if err := os.MkdirAll(authorsDir, 0755); err == nil {
				imgBytes, downloadErr := providers.DownloadURL(r.Context(), details.Image)
				if downloadErr == nil {
					destFile := filepath.Join(authorsDir, authorID+".jpg")
					if !utils.IsSafeFilePath(db, cfg.MetadataPath, destFile) {
						log.Warnf("[MatchAuthor] Blocked unsafe author image path traversal: %s", destFile)
						http.Error(w, "Forbidden", http.StatusForbidden)
						return
					}
					if writeErr := os.WriteFile(destFile, imgBytes, 0644); writeErr == nil {
						localImagePath = "authors/" + authorID + ".jpg"
					} else {
						log.Warnf("[MatchAuthor] Warning: failed to write author image file: %v", writeErr)
					}
				} else {
					log.Warnf("[MatchAuthor] Warning: failed to download author image: %v", downloadErr)
				}
			} else {
				log.Warnf("[MatchAuthor] Warning: failed to create authors metadata dir: %v", err)
			}
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		nowStr := time.Now().Format("2006-01-02 15:04:05.000")
		lastFirst := utils.NameToLastFirst(details.Name)

		if localImagePath != "" {
			_, err = tx.Exec(`
				UPDATE authors
				SET name = ?, lastFirst = ?, asin = ?, description = ?, imagePath = ?, updatedAt = ?
				WHERE id = ?
			`, details.Name, lastFirst, details.ASIN, details.Description, localImagePath, nowStr, authorID)
		} else {
			_, err = tx.Exec(`
				UPDATE authors
				SET name = ?, lastFirst = ?, asin = ?, description = ?, updatedAt = ?
				WHERE id = ?
			`, details.Name, lastFirst, details.ASIN, details.Description, nowStr, authorID)
		}

		if err != nil {
			http.Error(w, "Failed to update database: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Update linked libraryItems caches
		rows, err := tx.Query("SELECT bookId FROM bookAuthors WHERE authorId = ?", authorID)
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
				var authorNames []string
				var authorLastFirsts []string

				arows, err := tx.Query(`
					SELECT a.name, a.lastFirst
					FROM authors a
					JOIN bookAuthors ba ON ba.authorId = a.id
					WHERE ba.bookId = ?
				`, bid)
				if err == nil {
					for arows.Next() {
						var aName, aLastFirst string
						if err := arows.Scan(&aName, &aLastFirst); err == nil {
							authorNames = append(authorNames, aName)
							authorLastFirsts = append(authorLastFirsts, aLastFirst)
						}
					}
					arows.Close()
				}

				authorNamesStr := strings.Join(authorNames, ", ")
				authorLastFirstsStr := strings.Join(authorLastFirsts, ", ")

				_, _ = tx.Exec(`
					UPDATE libraryItems
					SET authorNamesFirstLast = ?, authorNamesLastFirst = ?, updatedAt = ?
					WHERE mediaId = ? AND mediaType = 'book'
				`, authorNamesStr, authorLastFirstsStr, nowStr, bid)
			}
		}

		if err := tx.Commit(); err != nil {
			http.Error(w, "Failed to commit transaction: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"updated": true}`))
	}
}

// handleGetLibraryItemByID resolves GET /api/items/{id}
func handleGetLibraryItemByID(db *sql.DB, itemID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/items/%s", itemID)

		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

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

		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		payload := map[string]interface{}{
			"id":           id,
			"ino":          ino,
			"libraryId":    libraryID,
			"folderId":     folderID,
			"path":         path,
			"relPath":      relPath,
			"isFile":       isFileVal != 0,
			"mtimeMs":      idb.ParseEpochMillis(mtimeStr),
			"ctimeMs":      idb.ParseEpochMillis(ctimeStr),
			"birthtimeMs":  idb.ParseEpochMillis(birthtimeStr),
			"addedAt":      idb.ParseEpochMillis(createdAtStr),
			"updatedAt":    idb.ParseEpochMillis(updatedAtStr),
			"isMissing":    isMissingVal != 0,
			"isInvalid":    isInvalidVal != 0,
			"mediaType":    mediaType,
			"size":         size,
			"libraryFiles": []interface{}{},
		}

		if mediaType == "book" {
			var bTitle string
			var bTitleIgnorePrefix, bSubtitle, bPublishedYear, bPublishedDate, bPublisher, bDescription, bIsbn, bAsin, bLanguage, bCoverPath sql.NullString
			var bDuration float64
			var bNarrators, bAudioFiles, bEbookFile, bChapters, bTags, bGenres, bLockedFields []byte
			var bExplicit, bAbridged sql.NullInt64

			err = db.QueryRow(`
				SELECT title, titleIgnorePrefix, subtitle, publishedYear, publishedDate, publisher, description, isbn, asin, language, explicit, abridged, coverPath, duration, narrators, audioFiles, ebookFile, chapters, tags, genres, lockedFields
				FROM books WHERE id = ?
			`, mediaID).Scan(
				&bTitle, &bTitleIgnorePrefix, &bSubtitle, &bPublishedYear, &bPublishedDate, &bPublisher, &bDescription, &bIsbn, &bAsin, &bLanguage, &bExplicit, &bAbridged, &bCoverPath, &bDuration, &bNarrators, &bAudioFiles, &bEbookFile, &bChapters, &bTags, &bGenres, &bLockedFields,
			)
			if err == nil {
				var tags []string
				_ = json.Unmarshal(bTags, &tags)
				if !user.IsAdminOrUp() {
					var explicit = bExplicit.Valid && bExplicit.Int64 != 0
					if explicit && !user.CanAccessExplicitContent {
						http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
						return
					}
					if !user.CheckCanAccessLibraryItemWithTags(tags) {
						http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
						return
					}
				}
				var genres []string
				_ = json.Unmarshal(bGenres, &genres)
				var audioFiles []map[string]interface{}
				_ = json.Unmarshal(bAudioFiles, &audioFiles)
				var ebook interface{}
				_ = json.Unmarshal(bEbookFile, &ebook)
				var chapters []interface{}
				_ = json.Unmarshal(bChapters, &chapters)
				var lockedFields []string
				if len(bLockedFields) > 0 {
					_ = json.Unmarshal(bLockedFields, &lockedFields)
				}
				if lockedFields == nil {
					lockedFields = []string{}
				}

				var authorNames []string
				var seriesNames []string
				var narratorNames []string
				_ = json.Unmarshal(bNarrators, &narratorNames)

				var authorsList []map[string]interface{} = []map[string]interface{}{}
				rows, err := db.Query("SELECT id, name FROM authors WHERE id IN (SELECT authorId FROM bookAuthors WHERE bookId = ?)", mediaID)
				if err == nil {
					defer rows.Close()
					for rows.Next() {
						var authorID, name string
						if err := rows.Scan(&authorID, &name); err == nil {
							authorsList = append(authorsList, map[string]interface{}{
								"id":   authorID,
								"name": name,
							})
							authorNames = append(authorNames, name)
						}
					}
				}

				var seriesList []map[string]interface{} = []map[string]interface{}{}
				srows, err := db.Query("SELECT s.id, s.name, bs.sequence FROM series s JOIN bookSeries bs ON s.id = bs.seriesId WHERE bs.bookId = ?", mediaID)
				if err == nil {
					defer srows.Close()
					for srows.Next() {
						var seriesID, name, sequence string
						if err := srows.Scan(&seriesID, &name, &sequence); err == nil {
							seriesList = append(seriesList, map[string]interface{}{
								"id":       seriesID,
								"name":     name,
								"sequence": sequence,
							})
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

				type AudiobookTrack struct {
					Index       int     `json:"index"`
					Exclude     bool    `json:"exclude"`
					Duration    float64 `json:"duration"`
					Codec       string  `json:"codec"`
					MimeType    string  `json:"mimeType"`
					StartOffset float64 `json:"startOffset"`
					Title       string  `json:"title"`
					Metadata    struct {
						Path     string `json:"path"`
						Filename string `json:"filename"`
						Size     int64  `json:"size"`
					} `json:"metadata"`
				}

				var rawTracks []AudiobookTrack
				_ = json.Unmarshal(bAudioFiles, &rawTracks)

				var tracks []map[string]interface{}
				var currentOffset float64 = 0.0
				for _, rt := range rawTracks {
					if rt.Exclude {
						continue
					}
					title := rt.Title
					if title == "" {
						title = rt.Metadata.Filename
					}
					tracks = append(tracks, map[string]interface{}{
						"index":       rt.Index,
						"startOffset": currentOffset,
						"duration":    rt.Duration,
						"title":       title,
						"mimeType":    rt.MimeType,
						"metadata": map[string]interface{}{
							"path":     rt.Metadata.Path,
							"filename": rt.Metadata.Filename,
							"size":     rt.Metadata.Size,
						},
					})
					currentOffset += rt.Duration
				}

				payload["media"] = map[string]interface{}{
					"id":            mediaID,
					"coverPath":     utils.NullIfEmpty(bCoverPath.String),
					"tags":          tags,
					"numTracks":     len(tracks),
					"numAudioFiles": len(audioFiles),
					"numChapters":   len(chapters),
					"duration":      bDuration,
					"size":          size,
					"ebookFormat":   ebookFormat,
					"audioFiles":    audioFiles,
					"tracks":        tracks,
					"ebookFile":     ebook,
					"chapters":      chapters,
					"metadata": map[string]interface{}{
						"title":             bTitle,
						"titleIgnorePrefix": bTitleIgnorePrefix.String,
						"subtitle":          utils.NullIfEmpty(bSubtitle.String),
						"authors":           authorsList,
						"authorName":        authorName,
						"authorNameLF":      utils.NameToLastFirst(authorName),
						"narrators":         narratorNames,
						"narratorName":      narratorName,
						"series":            seriesList,
						"seriesName":        seriesName,
						"genres":            genres,
						"publishedYear":     utils.NullIfEmpty(bPublishedYear.String),
						"publishedDate":     utils.NullIfEmpty(bPublishedDate.String),
						"publisher":         utils.NullIfEmpty(bPublisher.String),
						"description":       utils.NullIfEmpty(bDescription.String),
						"isbn":              utils.NullIfEmpty(bIsbn.String),
						"asin":              utils.NullIfEmpty(bAsin.String),
						"language":          utils.NullIfEmpty(bLanguage.String),
						"explicit":          bExplicit.Valid && bExplicit.Int64 != 0,
						"abridged":          bAbridged.Valid && bAbridged.Int64 != 0,
						"lockedFields":      lockedFields,
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

				// Fetch other versions (items with the same title)
				var otherVersions []map[string]interface{} = []map[string]interface{}{}
				vrows, err := db.Query(`
					SELECT li.id, b.title, b.subtitle, b.narrators, b.duration, b.coverPath
					FROM libraryItems li
					JOIN books b ON li.mediaId = b.id AND li.mediaType = 'book'
					WHERE li.libraryId = ? AND li.id != ? AND LOWER(b.title) = LOWER(?)
				`, libraryID, itemID, bTitle)
				if err == nil {
					defer vrows.Close()
					for vrows.Next() {
						var vID, vTitle string
						var vSubtitle, vCoverPath sql.NullString
						var vNarrators []byte
						var vDuration float64
						if err := vrows.Scan(&vID, &vTitle, &vSubtitle, &vNarrators, &vDuration, &vCoverPath); err == nil {
							var narrators []string
							_ = json.Unmarshal(vNarrators, &narrators)

							otherVersions = append(otherVersions, map[string]interface{}{
								"id":        vID,
								"title":     vTitle,
								"subtitle":  vSubtitle.String,
								"narrators": narrators,
								"duration":  vDuration,
								"coverPath": vCoverPath.String,
							})
						}
					}
				}
				payload["otherVersions"] = otherVersions
			} else {
				log.Warnf("[Go Warning] Failed to scan book with id %s: %v", mediaID, err)
			}
		} else if mediaType == "podcast" {
			var pTitle, pAuthor, pDescription, pLanguage, pPodcastType, pCoverPath sql.NullString
			var pExplicit sql.NullInt64
			var pTags, pGenres, pLockedFields []byte
			var autoDownloadVal, maxKeepVal, maxNewVal, autoDeleteVal int
			var scheduleVal sql.NullString

			err = db.QueryRow(`
				SELECT title, author, description, language, podcastType, explicit, coverPath, tags, genres, lockedFields,
				       autoDownloadEpisodes, autoDownloadSchedule, maxEpisodesToKeep, maxNewEpisodesToDownload, autoDeletePlayed
				FROM podcasts WHERE id = ?
			`, mediaID).Scan(
				&pTitle, &pAuthor, &pDescription, &pLanguage, &pPodcastType, &pExplicit, &pCoverPath, &pTags, &pGenres, &pLockedFields,
				&autoDownloadVal, &scheduleVal, &maxKeepVal, &maxNewVal, &autoDeleteVal,
			)
			if err == nil {
				var tags []string
				_ = json.Unmarshal(pTags, &tags)
				if !user.IsAdminOrUp() {
					var explicit = pExplicit.Valid && pExplicit.Int64 != 0
					if explicit && !user.CanAccessExplicitContent {
						http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
						return
					}
					if !user.CheckCanAccessLibraryItemWithTags(tags) {
						http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
						return
					}
				}
				var genres []string
				_ = json.Unmarshal(pGenres, &genres)
				var lockedFields []string
				if len(pLockedFields) > 0 {
					_ = json.Unmarshal(pLockedFields, &lockedFields)
				}
				if lockedFields == nil {
					lockedFields = []string{}
				}

				hasPubDate := hasColumn(r.Context(), db, "podcastEpisodes", "pubDate")
				hasDesc := hasColumn(r.Context(), db, "podcastEpisodes", "description")
				hasSeason := hasColumn(r.Context(), db, "podcastEpisodes", "season")
				hasEp := hasColumn(r.Context(), db, "podcastEpisodes", "episode")
				hasEpType := hasColumn(r.Context(), db, "podcastEpisodes", "episodeType")

				epQuery := "SELECT id, title, audioFile"
				if hasPubDate {
					epQuery += ", pubDate"
				}
				if hasDesc {
					epQuery += ", description"
				}
				if hasSeason {
					epQuery += ", season"
				}
				if hasEp {
					epQuery += ", episode"
				}
				if hasEpType {
					epQuery += ", episodeType"
				}
				epQuery += " FROM podcastEpisodes WHERE podcastId = ?"

				rows, err := db.Query(epQuery, mediaID)
				var episodes []map[string]interface{}
				if err == nil {
					defer rows.Close()
					for rows.Next() {
						var epID, epTitle, audioFileStr string
						var pubDateVal, descVal, seasonVal, epVal, epTypeVal sql.NullString

						dest := []interface{}{&epID, &epTitle, &audioFileStr}
						if hasPubDate {
							dest = append(dest, &pubDateVal)
						}
						if hasDesc {
							dest = append(dest, &descVal)
						}
						if hasSeason {
							dest = append(dest, &seasonVal)
						}
						if hasEp {
							dest = append(dest, &epVal)
						}
						if hasEpType {
							dest = append(dest, &epTypeVal)
						}

						if err := rows.Scan(dest...); err == nil {
							var af map[string]interface{}
							_ = json.Unmarshal([]byte(audioFileStr), &af)

							epMap := map[string]interface{}{
								"id":        epID,
								"title":     epTitle,
								"audioFile": af,
							}
							if hasPubDate && pubDateVal.Valid {
								epMap["pubDate"] = pubDateVal.String
							}
							if hasDesc && descVal.Valid {
								epMap["description"] = descVal.String
							}
							if hasSeason && seasonVal.Valid {
								epMap["season"] = seasonVal.String
							}
							if hasEp && epVal.Valid {
								epMap["episode"] = epVal.String
							}
							if hasEpType && epTypeVal.Valid {
								epMap["episodeType"] = epTypeVal.String
							}

							if af != nil {
								if dur, ok := af["duration"]; ok {
									epMap["duration"] = dur
								}
								if meta, ok := af["metadata"].(map[string]interface{}); ok {
									if sz, ok := meta["size"]; ok {
										epMap["size"] = sz
									}
								}
							}

							episodes = append(episodes, epMap)
						}
					}
				}

				payload["media"] = map[string]interface{}{
					"id":                       mediaID,
					"coverPath":                utils.NullIfEmpty(pCoverPath.String),
					"tags":                     tags,
					"episodes":                 episodes,
					"autoDownloadEpisodes":     autoDownloadVal == 1,
					"autoDownloadSchedule":     scheduleVal.String,
					"maxEpisodesToKeep":        maxKeepVal,
					"maxNewEpisodesToDownload": maxNewVal,
					"autoDeletePlayed":         autoDeleteVal == 1,
					"metadata": map[string]interface{}{
						"title":                    pTitle.String,
						"author":                   pAuthor.String,
						"description":              utils.NullIfEmpty(pDescription.String),
						"language":                 utils.NullIfEmpty(pLanguage.String),
						"podcastType":              utils.NullIfEmpty(pPodcastType.String),
						"explicit":                 pExplicit.Valid && pExplicit.Int64 != 0,
						"genres":                   genres,
						"lockedFields":             lockedFields,
						"autoDownloadEpisodes":     autoDownloadVal == 1,
						"autoDownloadSchedule":     scheduleVal.String,
						"maxEpisodesToKeep":        maxKeepVal,
						"maxNewEpisodesToDownload": maxNewVal,
						"autoDeletePlayed":         autoDeleteVal == 1,
					},
				}
			} else {
				log.Warnf("[Go Warning] Failed to scan podcast with id %s: %v", mediaID, err)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

// handleServeEbook serves the EPUB/PDF ebook file
func handleServeEbook(db *sql.DB, itemID string, fileID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/items/%s/ebook (fileID=%s)", itemID, fileID)

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
			log.Warnf("[Go] Ebook file not found: %s", filePath)
			http.NotFound(w, r)
			return
		}

		if !utils.IsSafeFilePath(db, MetadataPath, filePath) {
			log.Warnf("[Go] Ebook file path traversal blocked: %s", filePath)
			http.Error(w, "Forbidden", http.StatusForbidden)
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

// handleUpdateLibraryItemByID resolves PATCH /api/items/{id}
func handleUpdateLibraryItemByID(db *sql.DB, itemID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] PATCH /api/items/%s", itemID)

		if strings.Contains(itemID, "..") || strings.Contains(itemID, "/") || strings.Contains(itemID, "\\") {
			http.Error(w, `{"error": "Invalid item ID"}`, http.StatusBadRequest)
			return
		}

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

		var mediaID, mediaType, libraryID string
		err := db.QueryRow("SELECT mediaId, mediaType, libraryId FROM libraryItems WHERE id = ?", itemID).Scan(&mediaID, &mediaType, &libraryID)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		var payload struct {
			Title                    string   `json:"title"`
			Subtitle                 string   `json:"subtitle"`
			Authors                  []string `json:"authors"`
			Narrators                []string `json:"narrators"`
			SeriesName               string   `json:"seriesName"`
			SeriesSequence           string   `json:"seriesSequence"`
			Publisher                string   `json:"publisher"`
			PublishedYear            string   `json:"publishedYear"`
			PublishedDate            string   `json:"publishedDate"`
			Description              string   `json:"description"`
			Isbn                     string   `json:"isbn"`
			Asin                     string   `json:"asin"`
			Language                 string   `json:"language"`
			Explicit                 bool     `json:"explicit"`
			Abridged                 bool     `json:"abridged"`
			Tags                     []string `json:"tags"`
			Genres                   []string `json:"genres"`
			LockedFields             []string `json:"lockedFields"`
			AutoDownloadEpisodes     *bool    `json:"autoDownloadEpisodes"`
			AutoDownloadSchedule     *string  `json:"autoDownloadSchedule"`
			MaxEpisodesToKeep        *int     `json:"maxEpisodesToKeep"`
			MaxNewEpisodesToDownload *int     `json:"maxNewEpisodesToDownload"`
			AutoDeletePlayed         *bool    `json:"autoDeletePlayed"`
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

		if mediaType == "book" {
			authorNamesFirstLast := strings.Join(payload.Authors, ", ")
			var lfs []string
			for _, a := range payload.Authors {
				lfs = append(lfs, utils.NameToLastFirst(a))
			}
			authorNamesLastFirst := strings.Join(lfs, ", ")

			narratorsJSON, _ := json.Marshal(payload.Narrators)
			tagsJSON, _ := json.Marshal(payload.Tags)
			genresJSON, _ := json.Marshal(payload.Genres)
			lockedFieldsJSON, _ := json.Marshal(payload.LockedFields)

			prefixes := idb.GetSortingPrefixes(db)
			titleIgnorePrefix := getTitleIgnorePrefixGo(payload.Title, prefixes)

			_, err = tx.Exec(`
				UPDATE books
				SET title = ?, titleIgnorePrefix = ?, subtitle = ?, publishedYear = ?, publishedDate = ?, publisher = ?, description = ?, isbn = ?, asin = ?, language = ?, explicit = ?, abridged = ?, narrators = ?, tags = ?, genres = ?, lockedFields = ?
				WHERE id = ?
			`, payload.Title, titleIgnorePrefix, payload.Subtitle, payload.PublishedYear, payload.PublishedDate, payload.Publisher, payload.Description, payload.Isbn, payload.Asin, payload.Language, boolToInt(payload.Explicit), boolToInt(payload.Abridged), narratorsJSON, tagsJSON, genresJSON, lockedFieldsJSON, mediaID)
			if err != nil {
				http.Error(w, "failed to update book: "+err.Error(), http.StatusInternalServerError)
				return
			}

			if idb.TableExistsTx(tx, "bookAuthors") {
				_, _ = tx.Exec("DELETE FROM bookAuthors WHERE bookId = ?", mediaID)
			}
			for _, author := range payload.Authors {
				trimmed := strings.TrimSpace(author)
				if trimmed == "" {
					continue
				}
				authorID := utils.UUIDStr()
				lastFirst := utils.NameToLastFirst(trimmed)
				_ = iscanner.InsertAuthor(tx, authorID, trimmed, lastFirst, libraryID)

				var existingAuthorID string
				_ = tx.QueryRow("SELECT id FROM authors WHERE name = ? AND libraryId = ?", trimmed, libraryID).Scan(&existingAuthorID)
				if existingAuthorID != "" {
					authorID = existingAuthorID
				}
				_ = iscanner.InsertBookAuthor(tx, mediaID, authorID)
			}

			if idb.TableExistsTx(tx, "bookSeries") {
				_, _ = tx.Exec("DELETE FROM bookSeries WHERE bookId = ?", mediaID)
			}
			if payload.SeriesName != "" {
				seriesID := utils.UUIDStr()
				_ = iscanner.InsertSeries(tx, seriesID, payload.SeriesName, libraryID)

				var existingSeriesID string
				_ = tx.QueryRow("SELECT id FROM series WHERE name = ? AND libraryId = ?", payload.SeriesName, libraryID).Scan(&existingSeriesID)
				if existingSeriesID != "" {
					seriesID = existingSeriesID
				}
				_ = iscanner.InsertBookSeries(tx, mediaID, seriesID, payload.SeriesSequence)
			}

			_, err = tx.Exec(`
				UPDATE libraryItems
				SET title = ?, titleIgnorePrefix = ?, authorNamesFirstLast = ?, authorNamesLastFirst = ?, updatedAt = ?
				WHERE id = ?
			`, payload.Title, titleIgnorePrefix, authorNamesFirstLast, authorNamesLastFirst, nowStr, itemID)
			if err != nil {
				http.Error(w, "failed to update library item: "+err.Error(), http.StatusInternalServerError)
				return
			}

		} else if mediaType == "podcast" {
			tagsJSON, _ := json.Marshal(payload.Tags)
			genresJSON, _ := json.Marshal(payload.Genres)
			lockedFieldsJSON, _ := json.Marshal(payload.LockedFields)
			var author string
			if len(payload.Authors) > 0 {
				author = payload.Authors[0]
			}

			prefixes := idb.GetSortingPrefixes(db)
			titleIgnorePrefix := getTitleIgnorePrefixGo(payload.Title, prefixes)

			var currAutoDownload, currMaxKeep, currMaxNew, currAutoDelete int
			var currSchedule sql.NullString
			_ = db.QueryRow("SELECT autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload, autoDeletePlayed, autoDownloadSchedule FROM podcasts WHERE id = ?", mediaID).Scan(&currAutoDownload, &currMaxKeep, &currMaxNew, &currAutoDelete, &currSchedule)

			autoDownloadVal := currAutoDownload
			if payload.AutoDownloadEpisodes != nil {
				autoDownloadVal = boolToInt(*payload.AutoDownloadEpisodes)
			}
			maxKeepVal := currMaxKeep
			if payload.MaxEpisodesToKeep != nil {
				maxKeepVal = *payload.MaxEpisodesToKeep
			}
			maxNewVal := currMaxNew
			if payload.MaxNewEpisodesToDownload != nil {
				maxNewVal = *payload.MaxNewEpisodesToDownload
			}
			autoDeleteVal := currAutoDelete
			if payload.AutoDeletePlayed != nil {
				autoDeleteVal = boolToInt(*payload.AutoDeletePlayed)
			}
			scheduleVal := currSchedule.String
			if payload.AutoDownloadSchedule != nil {
				scheduleVal = *payload.AutoDownloadSchedule
			}

			_, err = tx.Exec(`
				UPDATE podcasts
				SET title = ?, titleIgnorePrefix = ?, author = ?, description = ?, language = ?, explicit = ?, tags = ?, genres = ?, lockedFields = ?,
				    autoDownloadEpisodes = ?, maxEpisodesToKeep = ?, maxNewEpisodesToDownload = ?, autoDeletePlayed = ?, autoDownloadSchedule = ?
				WHERE id = ?
			`, payload.Title, titleIgnorePrefix, author, payload.Description, payload.Language, boolToInt(payload.Explicit), tagsJSON, genresJSON, lockedFieldsJSON,
				autoDownloadVal, maxKeepVal, maxNewVal, autoDeleteVal, scheduleVal, mediaID)
			if err != nil {
				http.Error(w, "failed to update podcast: "+err.Error(), http.StatusInternalServerError)
				return
			}

			_, err = tx.Exec(`
				UPDATE libraryItems
				SET title = ?, titleIgnorePrefix = ?, authorNamesFirstLast = ?, authorNamesLastFirst = ?, updatedAt = ?
				WHERE id = ?
			`, payload.Title, titleIgnorePrefix, author, author, nowStr, itemID)
			if err != nil {
				http.Error(w, "failed to update library item: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}

		err = tx.Commit()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		srvSettings, srvErr := idb.GetServerSettings(db)
		var metadataPath string
		if srvErr == nil && srvSettings != nil && srvSettings.MetadataMarkdownWithItem {
			var itemPath string
			var isFile int
			dbErr := db.QueryRow("SELECT path, isFile FROM libraryItems WHERE id = ?", itemID).Scan(&itemPath, &isFile)
			if dbErr == nil && itemPath != "" {
				folder := itemPath
				if isFile != 0 {
					folder = filepath.Dir(itemPath)
				}
				metadataPath = filepath.Join(folder, "metadata.json")
			}
		} else {
			// Save in centralized metadata folder
			itemDir := filepath.Join(MetadataPath, "items", itemID)
			_ = os.MkdirAll(itemDir, 0755)
			metadataPath = filepath.Join(itemDir, "metadata.json")
		}

		if metadataPath != "" && utils.IsSafeFilePath(db, MetadataPath, metadataPath) {
			metaData := map[string]interface{}{
				"title":         payload.Title,
				"subtitle":      payload.Subtitle,
				"authors":       payload.Authors,
				"narrators":     payload.Narrators,
				"publisher":     payload.Publisher,
				"publishedYear": payload.PublishedYear,
				"publishedDate": payload.PublishedDate,
				"description":   payload.Description,
				"isbn":          payload.Isbn,
				"asin":          payload.Asin,
				"language":      payload.Language,
				"explicit":      payload.Explicit,
				"abridged":      payload.Abridged,
				"tags":          payload.Tags,
				"genres":        payload.Genres,
				"lockedFields":  payload.LockedFields,
			}
			metaJSON, marshalErr := json.MarshalIndent(metaData, "", "  ")
			if marshalErr == nil {
				_ = os.WriteFile(metadataPath, metaJSON, 0644)
			}
		}

		if isocket.GlobalAuth != nil {
			if minItem, err := idb.GetLibraryItemMinifiedByID(db, itemID); err == nil {
				EmitLibraryItemEvent("item_updated", minItem)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true}`))
	}
}

// handleUpdateAuthor resolves PATCH /api/authors/{id}
func handleUpdateAuthor(db *sql.DB, authorID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] PATCH /api/authors/%s", authorID)

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
			Name        string `json:"name"`
			LastFirst   string `json:"lastFirst"`
			Asin        string `json:"asin"`
			Description string `json:"description"`
		}

		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		srvSettings, srvErr := idb.GetServerSettings(db)

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		nowStr := time.Now().Format("2006-01-02 15:04:05.000")

		// Update the author table
		_, err = tx.Exec(`
			UPDATE authors
			SET name = ?, lastFirst = ?, asin = ?, description = ?, updatedAt = ?
			WHERE id = ?
		`, payload.Name, payload.LastFirst, payload.Asin, payload.Description, nowStr, authorID)
		if err != nil {
			http.Error(w, "failed to update author: "+err.Error(), http.StatusInternalServerError)
			return
		}

		type BookUpdate struct {
			bid          string
			itemID       string
			authorNames  []string
			metadataPath string
		}
		var booksToUpdate []BookUpdate

		// Fetch all books linked to this author in bookAuthors
		rows, err := tx.Query("SELECT bookId FROM bookAuthors WHERE authorId = ?", authorID)
		if err == nil {
			var bookIDs []string
			for rows.Next() {
				var bid string
				if err := rows.Scan(&bid); err == nil {
					bookIDs = append(bookIDs, bid)
				}
			}
			rows.Close()

			// For each book, fetch all authors, join them, and update libraryItems table
			for _, bid := range bookIDs {
				var authorNames []string
				var authorLastFirsts []string

				arows, err := tx.Query(`
					SELECT a.name, a.lastFirst
					FROM authors a
					JOIN bookAuthors ba ON ba.authorId = a.id
					WHERE ba.bookId = ?
				`, bid)
				if err == nil {
					for arows.Next() {
						var aName, aLastFirst string
						if err := arows.Scan(&aName, &aLastFirst); err == nil {
							authorNames = append(authorNames, aName)
							authorLastFirsts = append(authorLastFirsts, aLastFirst)
						}
					}
					arows.Close()
				}

				authorNamesStr := strings.Join(authorNames, ", ")
				authorLastFirstsStr := strings.Join(authorLastFirsts, ", ")

				_, _ = tx.Exec(`
					UPDATE libraryItems
					SET authorNamesFirstLast = ?, authorNamesLastFirst = ?, updatedAt = ?
					WHERE mediaId = ? AND mediaType = 'book'
				`, authorNamesStr, authorLastFirstsStr, nowStr, bid)

				var itemID string
				_ = tx.QueryRow("SELECT id FROM libraryItems WHERE mediaId = ? AND mediaType = 'book'", bid).Scan(&itemID)

				var metadataPath string
				if srvErr == nil && srvSettings != nil && srvSettings.MetadataMarkdownWithItem {
					var itemPath string
					var isFile int
					dbErr := tx.QueryRow("SELECT path, isFile FROM libraryItems WHERE mediaId = ? AND mediaType = 'book'", bid).Scan(&itemPath, &isFile)
					if dbErr == nil && itemPath != "" {
						folder := itemPath
						if isFile != 0 {
							folder = filepath.Dir(itemPath)
						}
						metadataPath = filepath.Join(folder, "metadata.json")
					}
				} else if itemID != "" {
					metadataPath = filepath.Join(MetadataPath, "items", itemID, "metadata.json")
				}

				booksToUpdate = append(booksToUpdate, BookUpdate{
					bid:          bid,
					itemID:       itemID,
					authorNames:  authorNames,
					metadataPath: metadataPath,
				})
			}
		}

		err = tx.Commit()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Perform post-commit disk writes and websocket events
		for _, b := range booksToUpdate {
			if b.metadataPath != "" && utils.IsSafeFilePath(db, MetadataPath, b.metadataPath) {
				if _, err := os.Stat(b.metadataPath); err == nil {
					var metadata map[string]interface{}
					if mBytes, err := os.ReadFile(b.metadataPath); err == nil {
						if json.Unmarshal(mBytes, &metadata) == nil {
							metadata["authors"] = b.authorNames
							if mJSON, err := json.MarshalIndent(metadata, "", "  "); err == nil {
								_ = os.WriteFile(b.metadataPath, mJSON, 0644)
							}
						}
					}
				}
			}

			// Emit real-time update
			if isocket.GlobalAuth != nil && b.itemID != "" {
				if minItem, err := idb.GetLibraryItemMinifiedByID(db, b.itemID); err == nil {
					EmitLibraryItemEvent("item_updated", minItem)
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true}`))
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

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// handleDeleteLibraryItemByID resolves DELETE /api/items/{id}
func handleDeleteLibraryItemByID(db *sql.DB, itemID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] DELETE /api/items/%s", itemID)

		if strings.Contains(itemID, "..") || strings.Contains(itemID, "/") || strings.Contains(itemID, "\\") {
			http.Error(w, `{"error": "Invalid item ID"}`, http.StatusBadRequest)
			return
		}

		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		if user.Type != "root" && user.Type != "admin" && !user.CanDelete {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		// Verify library item exists and get mediaId & mediaType
		var mediaID, mediaType string
		err := db.QueryRow("SELECT mediaId, mediaType FROM libraryItems WHERE id = ?", itemID).Scan(&mediaID, &mediaType)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			log.Errorf("[Delete Item] Failed to begin transaction: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		// Delete from playlistMediaItems
		_, _ = tx.Exec("DELETE FROM playlistMediaItems WHERE mediaItemId = ?", itemID)

		// Delete from mediaProgresses
		_, _ = tx.Exec("DELETE FROM mediaProgresses WHERE mediaItemId = ?", itemID)

		// Delete from libraryItems
		_, err = tx.Exec("DELETE FROM libraryItems WHERE id = ?", itemID)
		if err != nil {
			log.Errorf("[Delete Item] Failed to delete library item: %v", err)
			http.Error(w, `{"error": "Database Error"}`, http.StatusInternalServerError)
			return
		}

		// Clean up book/podcast if they are no longer referenced in libraryItems
		if mediaType == "book" {
			_, _ = tx.Exec("DELETE FROM bookAuthors WHERE bookId = ?", mediaID)
			_, _ = tx.Exec("DELETE FROM bookSeries WHERE bookId = ?", mediaID)
			_, _ = tx.Exec("DELETE FROM books WHERE id = ? AND id NOT IN (SELECT mediaId FROM libraryItems WHERE mediaType = 'book')", mediaID)
		} else if mediaType == "podcast" {
			_, _ = tx.Exec("DELETE FROM podcastEpisodes WHERE podcastId = ?", mediaID)
			_, _ = tx.Exec("DELETE FROM podcasts WHERE id = ? AND id NOT IN (SELECT mediaId FROM libraryItems WHERE mediaType = 'podcast')", mediaID)
		}

		if err := tx.Commit(); err != nil {
			log.Errorf("[Delete Item] Failed to commit transaction: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success": true}`))
	}
}
