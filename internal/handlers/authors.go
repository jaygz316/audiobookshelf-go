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
