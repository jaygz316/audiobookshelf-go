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

type BatchUpdateMediaPayload struct {
	Title          *string   `json:"title"`
	Subtitle       *string   `json:"subtitle"`
	Authors        *[]string `json:"authors"`
	Narrators      *[]string `json:"narrators"`
	SeriesName     *string   `json:"seriesName"`
	SeriesSequence *string   `json:"seriesSequence"`
	Publisher      *string   `json:"publisher"`
	PublishedYear  *string   `json:"publishedYear"`
	PublishedDate  *string   `json:"publishedDate"`
	Description    *string   `json:"description"`
	Isbn           *string   `json:"isbn"`
	Asin           *string   `json:"asin"`
	Language       *string   `json:"language"`
	Explicit       *bool     `json:"explicit"`
	Abridged       *bool     `json:"abridged"`
	Tags           *[]string `json:"tags"`
	Genres         *[]string `json:"genres"`
}

type BatchUpdateItem struct {
	ID           string                  `json:"id"`
	MediaPayload BatchUpdateMediaPayload `json:"mediaPayload"`
}

func handleBatchUpdateLibraryItems(db *sql.DB, cfg *core.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST /api/items/batch/update")

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

		var payload []BatchUpdateItem
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
		prefixes := idb.GetSortingPrefixes(db)

		type updatedItemInfo struct {
			itemID    string
			mediaID   string
			mediaType string
			payload   BatchUpdateMediaPayload
		}
		var updatedItems []updatedItemInfo

		for _, item := range payload {
			var mediaID, mediaType, libraryID string
			err = tx.QueryRow("SELECT COALESCE(mediaId, ''), COALESCE(mediaType, ''), COALESCE(libraryId, '') FROM libraryItems WHERE id = ?", item.ID).Scan(&mediaID, &mediaType, &libraryID)
			if err != nil {
				log.Warnf("[Go] Batch edit: item %s not found", item.ID)
				continue
			}

			if mediaType == "book" {
				var currentTitle, currentSubtitle, currentPublishedYear, currentPublishedDate, currentPublisher, currentDescription, currentIsbn, currentAsin, currentLanguage, currentNarratorsRaw, currentTagsRaw, currentGenresRaw string
				var currentExplicitVal, currentAbridgedVal int
				err = tx.QueryRow(`
					SELECT COALESCE(title, ''), COALESCE(subtitle, ''), COALESCE(publishedYear, ''), COALESCE(publishedDate, ''), COALESCE(publisher, ''), COALESCE(description, ''), COALESCE(isbn, ''), COALESCE(asin, ''), COALESCE(language, ''), explicit, abridged, COALESCE(narrators, '[]'), COALESCE(tags, '[]'), COALESCE(genres, '[]')
					FROM books WHERE id = ?
				`, mediaID).Scan(&currentTitle, &currentSubtitle, &currentPublishedYear, &currentPublishedDate, &currentPublisher, &currentDescription, &currentIsbn, &currentAsin, &currentLanguage, &currentExplicitVal, &currentAbridgedVal, &currentNarratorsRaw, &currentTagsRaw, &currentGenresRaw)
				if err != nil {
					log.Errorf("[Go] Batch edit: book media %s not found: %v", mediaID, err)
					continue
				}

				var currentNarrators, currentTags, currentGenres []string
				_ = json.Unmarshal([]byte(currentNarratorsRaw), &currentNarrators)
				_ = json.Unmarshal([]byte(currentTagsRaw), &currentTags)
				_ = json.Unmarshal([]byte(currentGenresRaw), &currentGenres)

				title := currentTitle
				if item.MediaPayload.Title != nil {
					title = *item.MediaPayload.Title
				}
				subtitle := currentSubtitle
				if item.MediaPayload.Subtitle != nil {
					subtitle = *item.MediaPayload.Subtitle
				}
				publishedYear := currentPublishedYear
				if item.MediaPayload.PublishedYear != nil {
					publishedYear = *item.MediaPayload.PublishedYear
				}
				publishedDate := currentPublishedDate
				if item.MediaPayload.PublishedDate != nil {
					publishedDate = *item.MediaPayload.PublishedDate
				}
				publisher := currentPublisher
				if item.MediaPayload.Publisher != nil {
					publisher = *item.MediaPayload.Publisher
				}
				description := currentDescription
				if item.MediaPayload.Description != nil {
					description = *item.MediaPayload.Description
				}
				isbn := currentIsbn
				if item.MediaPayload.Isbn != nil {
					isbn = *item.MediaPayload.Isbn
				}
				asin := currentAsin
				if item.MediaPayload.Asin != nil {
					asin = *item.MediaPayload.Asin
				}
				language := currentLanguage
				if item.MediaPayload.Language != nil {
					language = *item.MediaPayload.Language
				}
				explicit := currentExplicitVal != 0
				if item.MediaPayload.Explicit != nil {
					explicit = *item.MediaPayload.Explicit
				}
				abridged := currentAbridgedVal != 0
				if item.MediaPayload.Abridged != nil {
					abridged = *item.MediaPayload.Abridged
				}

				narrators := currentNarrators
				if item.MediaPayload.Narrators != nil {
					narrators = *item.MediaPayload.Narrators
				}
				tags := currentTags
				if item.MediaPayload.Tags != nil {
					tags = *item.MediaPayload.Tags
				}
				genres := currentGenres
				if item.MediaPayload.Genres != nil {
					genres = *item.MediaPayload.Genres
				}

				titleIgnorePrefix := getTitleIgnorePrefixGo(title, prefixes)
				narratorsJSON, _ := json.Marshal(narrators)
				tagsJSON, _ := json.Marshal(tags)
				genresJSON, _ := json.Marshal(genres)

				_, err = tx.Exec(`
					UPDATE books
					SET title = ?, titleIgnorePrefix = ?, subtitle = ?, publishedYear = ?, publishedDate = ?, publisher = ?, description = ?, isbn = ?, asin = ?, language = ?, explicit = ?, abridged = ?, narrators = ?, tags = ?, genres = ?
					WHERE id = ?
				`, title, titleIgnorePrefix, subtitle, publishedYear, publishedDate, publisher, description, isbn, asin, language, boolToInt(explicit), boolToInt(abridged), narratorsJSON, tagsJSON, genresJSON, mediaID)
				if err != nil {
					return
				}

				// Authors
				var authorNames []string
				if item.MediaPayload.Authors != nil {
					authorNames = *item.MediaPayload.Authors
					if idb.TableExistsTx(tx, "bookAuthors") {
						_, _ = tx.Exec("DELETE FROM bookAuthors WHERE bookId = ?", mediaID)
					}
					for _, author := range authorNames {
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
				} else {
					rows, err := tx.Query(`
						SELECT COALESCE(a.name, '') FROM authors a
						JOIN bookAuthors ba ON a.id = ba.authorId
						WHERE ba.bookId = ?
					`, mediaID)
					if err == nil {
						defer rows.Close()
						for rows.Next() {
							var name string
							if errScan := rows.Scan(&name); errScan == nil {
								authorNames = append(authorNames, name)
							}
						}
					}
				}

				authorNamesFirstLast := strings.Join(authorNames, ", ")
				var lfs []string
				for _, a := range authorNames {
					lfs = append(lfs, utils.NameToLastFirst(a))
				}
				authorNamesLastFirst := strings.Join(lfs, ", ")

				// Series
				if item.MediaPayload.SeriesName != nil {
					seriesName := *item.MediaPayload.SeriesName
					seriesSeq := ""
					if item.MediaPayload.SeriesSequence != nil {
						seriesSeq = *item.MediaPayload.SeriesSequence
					} else {
						_ = tx.QueryRow("SELECT COALESCE(sequence, '') FROM bookSeries WHERE bookId = ?", mediaID).Scan(&seriesSeq)
					}

					if idb.TableExistsTx(tx, "bookSeries") {
						_, _ = tx.Exec("DELETE FROM bookSeries WHERE bookId = ?", mediaID)
					}
					if seriesName != "" {
						seriesID := utils.UUIDStr()
						_ = iscanner.InsertSeries(tx, seriesID, seriesName, libraryID)

						var existingSeriesID string
						_ = tx.QueryRow("SELECT id FROM series WHERE name = ? AND libraryId = ?", seriesName, libraryID).Scan(&existingSeriesID)
						if existingSeriesID != "" {
							seriesID = existingSeriesID
						}
						_ = iscanner.InsertBookSeries(tx, mediaID, seriesID, seriesSeq)
					}
				}

				_, err = tx.Exec(`
					UPDATE libraryItems
					SET title = ?, titleIgnorePrefix = ?, authorNamesFirstLast = ?, authorNamesLastFirst = ?, updatedAt = ?
					WHERE id = ?
				`, title, titleIgnorePrefix, authorNamesFirstLast, authorNamesLastFirst, nowStr, item.ID)
				if err != nil {
					return
				}

			} else if mediaType == "podcast" {
				var currentTitle, currentAuthor, currentDescription, currentLanguage, currentTagsRaw, currentGenresRaw string
				var currentExplicitVal int
				err = tx.QueryRow(`
					SELECT COALESCE(title, ''), COALESCE(author, ''), COALESCE(description, ''), COALESCE(language, ''), explicit, COALESCE(tags, '[]'), COALESCE(genres, '[]')
					FROM podcasts WHERE id = ?
				`, mediaID).Scan(&currentTitle, &currentAuthor, &currentDescription, &currentLanguage, &currentExplicitVal, &currentTagsRaw, &currentGenresRaw)
				if err != nil {
					log.Errorf("[Go] Batch edit: podcast media %s not found: %v", mediaID, err)
					continue
				}

				var currentTags, currentGenres []string
				_ = json.Unmarshal([]byte(currentTagsRaw), &currentTags)
				_ = json.Unmarshal([]byte(currentGenresRaw), &currentGenres)

				title := currentTitle
				if item.MediaPayload.Title != nil {
					title = *item.MediaPayload.Title
				}
				description := currentDescription
				if item.MediaPayload.Description != nil {
					description = *item.MediaPayload.Description
				}
				language := currentLanguage
				if item.MediaPayload.Language != nil {
					language = *item.MediaPayload.Language
				}
				explicit := currentExplicitVal != 0
				if item.MediaPayload.Explicit != nil {
					explicit = *item.MediaPayload.Explicit
				}
				tags := currentTags
				if item.MediaPayload.Tags != nil {
					tags = *item.MediaPayload.Tags
				}
				genres := currentGenres
				if item.MediaPayload.Genres != nil {
					genres = *item.MediaPayload.Genres
				}

				author := currentAuthor
				if item.MediaPayload.Authors != nil && len(*item.MediaPayload.Authors) > 0 {
					author = (*item.MediaPayload.Authors)[0]
				}

				titleIgnorePrefix := getTitleIgnorePrefixGo(title, prefixes)
				tagsJSON, _ := json.Marshal(tags)
				genresJSON, _ := json.Marshal(genres)

				_, err = tx.Exec(`
					UPDATE podcasts
					SET title = ?, titleIgnorePrefix = ?, author = ?, description = ?, language = ?, explicit = ?, tags = ?, genres = ?
					WHERE id = ?
				`, title, titleIgnorePrefix, author, description, language, boolToInt(explicit), tagsJSON, genresJSON, mediaID)
				if err != nil {
					return
				}

				_, err = tx.Exec(`
					UPDATE libraryItems
					SET title = ?, titleIgnorePrefix = ?, authorNamesFirstLast = ?, authorNamesLastFirst = ?, updatedAt = ?
					WHERE id = ?
				`, title, titleIgnorePrefix, author, author, nowStr, item.ID)
				if err != nil {
					return
				}
			}

			updatedItems = append(updatedItems, updatedItemInfo{
				itemID:    item.ID,
				mediaID:   mediaID,
				mediaType: mediaType,
				payload:   item.MediaPayload,
			})
		}

		err = tx.Commit()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		srvSettings, srvErr := idb.GetServerSettings(db)
		for _, info := range updatedItems {
			var metadataPath string
			var dbErr error
			if srvErr == nil && srvSettings != nil && srvSettings.MetadataMarkdownWithItem {
				var itemPath string
				var isFile int
				dbErr = db.QueryRow("SELECT COALESCE(path, ''), isFile FROM libraryItems WHERE id = ?", info.itemID).Scan(&itemPath, &isFile)
				if dbErr == nil && itemPath != "" {
					folder := itemPath
					if isFile != 0 {
						folder = filepath.Dir(itemPath)
					}
					metadataPath = filepath.Join(folder, "metadata.json")
				}
			} else {
				itemDir := filepath.Join(MetadataPath, "items", info.itemID)
				_ = os.MkdirAll(itemDir, 0755)
				metadataPath = filepath.Join(itemDir, "metadata.json")
			}

			if metadataPath != "" {
				if info.mediaType == "book" {
					var title, subtitle, publisher, publishedYear, publishedDate, description, isbn, asin, language, narratorsRaw, tagsRaw, genresRaw string
					var explicitVal, abridgedVal int
					dbErr = db.QueryRow(`
							SELECT COALESCE(title, ''), COALESCE(subtitle, ''), COALESCE(publishedYear, ''), COALESCE(publishedDate, ''), COALESCE(publisher, ''), COALESCE(description, ''), COALESCE(isbn, ''), COALESCE(asin, ''), COALESCE(language, ''), explicit, abridged, COALESCE(narrators, '[]'), COALESCE(tags, '[]'), COALESCE(genres, '[]')
							FROM books WHERE id = ?
						`, info.mediaID).Scan(&title, &subtitle, &publishedYear, &publishedDate, &publisher, &description, &isbn, &asin, &language, &explicitVal, &abridgedVal, &narratorsRaw, &tagsRaw, &genresRaw)
					if dbErr == nil {
						var narrators, tags, genres []string
						_ = json.Unmarshal([]byte(narratorsRaw), &narrators)
						_ = json.Unmarshal([]byte(tagsRaw), &tags)
						_ = json.Unmarshal([]byte(genresRaw), &genres)

						var authors []string
						rows, authorErr := db.Query(`
								SELECT COALESCE(a.name, '') FROM authors a
								JOIN bookAuthors ba ON a.id = ba.authorId
								WHERE ba.bookId = ?
							`, info.mediaID)
						if authorErr == nil {
							defer rows.Close()
							for rows.Next() {
								var name string
								if errScan := rows.Scan(&name); errScan == nil {
									authors = append(authors, name)
								}
							}
						}

						metaData := map[string]interface{}{
							"title":         title,
							"subtitle":      subtitle,
							"authors":       authors,
							"narrators":     narrators,
							"publisher":     publisher,
							"publishedYear": publishedYear,
							"publishedDate": publishedDate,
							"description":   description,
							"isbn":          isbn,
							"asin":          asin,
							"language":      language,
							"explicit":      explicitVal != 0,
							"abridged":      abridgedVal != 0,
							"tags":          tags,
							"genres":        genres,
						}
						metaJSON, marshalErr := json.MarshalIndent(metaData, "", "  ")
						if marshalErr == nil {
							_ = os.WriteFile(metadataPath, metaJSON, 0644)
						}
					}
				} else if info.mediaType == "podcast" {
					var title, author, description, language, tagsRaw, genresRaw string
					var explicitVal int
					dbErr = db.QueryRow(`
							SELECT COALESCE(title, ''), COALESCE(author, ''), COALESCE(description, ''), COALESCE(language, ''), explicit, COALESCE(tags, '[]'), COALESCE(genres, '[]')
							FROM podcasts WHERE id = ?
						`, info.mediaID).Scan(&title, &author, &description, &language, &explicitVal, &tagsRaw, &genresRaw)
					if dbErr == nil {
						var tags, genres []string
						_ = json.Unmarshal([]byte(tagsRaw), &tags)
						_ = json.Unmarshal([]byte(genresRaw), &genres)

						metaData := map[string]interface{}{
							"title":       title,
							"author":      author,
							"description": description,
							"language":    language,
							"explicit":    explicitVal != 0,
							"tags":        tags,
							"genres":      genres,
						}
						metaJSON, marshalErr := json.MarshalIndent(metaData, "", "  ")
						if marshalErr == nil {
							_ = os.WriteFile(metadataPath, metaJSON, 0644)
						}
					}
				}
			}

			if isocket.GlobalAuth != nil {
				if minItem, err := idb.GetLibraryItemMinifiedByID(db, info.itemID); err == nil {
					EmitLibraryItemEvent("item_updated", minItem)
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true}`))
	}
}
