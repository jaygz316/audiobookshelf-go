package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	iscanner "audiobookshelf/internal/scanner"
	isocket "audiobookshelf/internal/socket"
	"audiobookshelf/internal/utils"
)

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
			SkipIntroDuration        *int     `json:"skipIntroDuration"`
			SkipOutroDuration        *int     `json:"skipOutroDuration"`
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

			var currAutoDownload, currMaxKeep, currMaxNew, currAutoDelete, currSkipIntro, currSkipOutro int
			var currSchedule sql.NullString
			_ = db.QueryRow("SELECT autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload, autoDeletePlayed, autoDownloadSchedule, skipIntroDuration, skipOutroDuration FROM podcasts WHERE id = ?", mediaID).Scan(&currAutoDownload, &currMaxKeep, &currMaxNew, &currAutoDelete, &currSchedule, &currSkipIntro, &currSkipOutro)

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
			skipIntroVal := currSkipIntro
			if payload.SkipIntroDuration != nil {
				skipIntroVal = *payload.SkipIntroDuration
			}
			skipOutroVal := currSkipOutro
			if payload.SkipOutroDuration != nil {
				skipOutroVal = *payload.SkipOutroDuration
			}

			_, err = tx.Exec(`
				UPDATE podcasts
				SET title = ?, titleIgnorePrefix = ?, author = ?, description = ?, language = ?, explicit = ?, tags = ?, genres = ?, lockedFields = ?,
				    autoDownloadEpisodes = ?, maxEpisodesToKeep = ?, maxNewEpisodesToDownload = ?, autoDeletePlayed = ?, autoDownloadSchedule = ?,
				    skipIntroDuration = ?, skipOutroDuration = ?
				WHERE id = ?
			`, payload.Title, titleIgnorePrefix, author, payload.Description, payload.Language, boolToInt(payload.Explicit), tagsJSON, genresJSON, lockedFieldsJSON,
				autoDownloadVal, maxKeepVal, maxNewVal, autoDeleteVal, scheduleVal, skipIntroVal, skipOutroVal, mediaID)
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

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
