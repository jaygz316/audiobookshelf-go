package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"audiobookshelf/internal/utils"
)

type LibraryItemDownloadInfo struct {
	Path    string
	RelPath string
	IsFile  bool
}

// GetLibraryItemDownloadInfo fetches file path, relPath, and isFile status for a library item.
func GetLibraryItemDownloadInfo(db *sql.DB, itemID string) (*LibraryItemDownloadInfo, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var info LibraryItemDownloadInfo
	var isFileVal sql.NullInt64
	var pathStr, relPathStr sql.NullString
	err := db.QueryRow("SELECT path, relPath, isFile FROM libraryItems WHERE id = ?", itemID).
		Scan(&pathStr, &relPathStr, &isFileVal)
	if err != nil {
		return nil, err
	}
	info.Path = pathStr.String
	info.RelPath = relPathStr.String
	info.IsFile = isFileVal.Valid && isFileVal.Int64 != 0
	return &info, nil
}

// GetCoverPath reads the media coverPath from books or podcasts table based on the library item ID.
func GetCoverPath(db *sql.DB, itemID string) (string, error) {
	if db == nil {
		return "", fmt.Errorf("database not initialized")
	}
	var mediaType, mediaID string
	err := db.QueryRow("SELECT mediaType, mediaId FROM libraryItems WHERE id = ?", itemID).Scan(&mediaType, &mediaID)
	if err != nil {
		return "", err
	}

	var coverPath sql.NullString
	if mediaType == "book" {
		err = db.QueryRow("SELECT coverPath FROM books WHERE id = ?", mediaID).Scan(&coverPath)
	} else if mediaType == "podcast" {
		err = db.QueryRow("SELECT coverPath FROM podcasts WHERE id = ?", mediaID).Scan(&coverPath)
	} else {
		return "", fmt.Errorf("unknown media type: %s", mediaType)
	}

	if err != nil {
		return "", err
	}

	if !coverPath.Valid {
		return "", nil
	}

	return coverPath.String, nil
}

type LibraryItemMinifiedJSON struct {
	ID               string      `json:"id"`
	Ino              string      `json:"ino"`
	OldLibraryItemID *string     `json:"oldLibraryItemId"`
	LibraryID        string      `json:"libraryId"`
	FolderID         string      `json:"folderId"`
	Path             string      `json:"path"`
	RelPath          string      `json:"relPath"`
	IsFile           bool        `json:"isFile"`
	MtimeMs          int64       `json:"mtimeMs"`
	CtimeMs          int64       `json:"ctimeMs"`
	BirthtimeMs      int64       `json:"birthtimeMs"`
	AddedAt          int64       `json:"addedAt"`
	UpdatedAt        int64       `json:"updatedAt"`
	IsMissing        bool        `json:"isMissing"`
	IsInvalid        bool        `json:"isInvalid"`
	MediaType        string      `json:"mediaType"`
	Media            interface{} `json:"media"`
	NumFiles         int         `json:"numFiles"`
	Size             int64       `json:"size"`
}

type BookMinifiedJSON struct {
	ID            string                `json:"id"`
	Metadata      *BookMetadataMinified `json:"metadata"`
	CoverPath     *string               `json:"coverPath"`
	Tags          []string              `json:"tags"`
	NumTracks     int                   `json:"numTracks"`
	NumAudioFiles int                   `json:"numAudioFiles"`
	NumChapters   int                   `json:"numChapters"`
	Duration      float64               `json:"duration"`
	Size          int64                 `json:"size"`
	EbookFormat   *string               `json:"ebookFormat"`
	AudioFiles    []interface{}         `json:"audioFiles,omitempty"`
}

type BookSeriesMinifiedJSON struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Sequence string `json:"sequence"`
}

type BookMetadataMinified struct {
	Title             string                    `json:"title"`
	TitleIgnorePrefix string                    `json:"titleIgnorePrefix"`
	Subtitle          *string                   `json:"subtitle"`
	AuthorName        string                    `json:"authorName"`
	AuthorNameLF      string                    `json:"authorNameLF"`
	NarratorName      string                    `json:"narratorName"`
	SeriesName        string                    `json:"seriesName"`
	SeriesSequence    *string                   `json:"seriesSequence"`
	Series            []*BookSeriesMinifiedJSON `json:"series"`
	Genres            []string                  `json:"genres"`
	PublishedYear     *string                   `json:"publishedYear"`
	PublishedDate     *string                   `json:"publishedDate"`
	Publisher         *string                   `json:"publisher"`
	Description       *string                   `json:"description"`
	Isbn              *string                   `json:"isbn"`
	Asin              *string                   `json:"asin"`
	Language          *string                   `json:"language"`
	Explicit          bool                      `json:"explicit"`
	Abridged          bool                      `json:"abridged"`
	LockedFields      []string                  `json:"lockedFields"`
}

type PodcastMinifiedJSON struct {
	ID                       string              `json:"id"`
	Metadata                 *PodcastMetadataMin `json:"metadata"`
	CoverPath                *string             `json:"coverPath"`
	Tags                     []string            `json:"tags"`
	NumEpisodes              int                 `json:"numEpisodes"`
	AutoDownloadEpisodes     bool                `json:"autoDownloadEpisodes"`
	AutoDownloadSchedule     *string             `json:"autoDownloadSchedule"`
	LastEpisodeCheck         *int64              `json:"lastEpisodeCheck"`
	MaxEpisodesToKeep        int                 `json:"maxEpisodesToKeep"`
	MaxNewEpisodesToDownload int                 `json:"maxNewEpisodesToDownload"`
	Size                     int64               `json:"size"`
	Episodes                 []interface{}       `json:"episodes,omitempty"`
}

type PodcastMetadataMin struct {
	Title             string   `json:"title"`
	TitleIgnorePrefix string   `json:"titleIgnorePrefix"`
	Author            *string  `json:"author"`
	Description       *string  `json:"description"`
	ReleaseDate       *string  `json:"releaseDate"`
	Genres            []string `json:"genres"`
	FeedURL           *string  `json:"feedUrl"`
	ImageURL          *string  `json:"imageUrl"`
	ItunesPageURL     *string  `json:"itunesPageUrl"`
	ItunesID          *string  `json:"itunesId"`
	ItunesArtistID    *string  `json:"itunesArtistId"`
	Explicit          bool     `json:"explicit"`
	Language          *string  `json:"language"`
	Type              *string  `json:"type"`
	LockedFields      []string `json:"lockedFields"`
}

// GetLibraryItemMinifiedByID retrieves a library item in its minified JSON form by ID.
func GetLibraryItemMinifiedByID(db *sql.DB, itemID string) (*LibraryItemMinifiedJSON, error) {
	var li LibraryItemMinifiedJSON
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
		return nil, err
	}

	li.ID = id
	li.Ino = ino
	li.LibraryID = libraryID
	li.FolderID = folderID
	li.Path = path
	li.RelPath = relPath
	li.IsFile = isFileVal != 0
	li.MtimeMs = parseEpochMillis(mtimeStr)
	li.CtimeMs = parseEpochMillis(ctimeStr)
	li.BirthtimeMs = parseEpochMillis(birthtimeStr)
	li.AddedAt = parseEpochMillis(createdAtStr)
	li.UpdatedAt = parseEpochMillis(updatedAtStr)
	li.IsMissing = isMissingVal != 0
	li.IsInvalid = isInvalidVal != 0
	li.MediaType = mediaType
	li.Size = size

	if mediaType == "book" {
		var bTitle, bTitleIgnorePrefix, bSubtitle, bPublishedYear, bPublishedDate, bPublisher, bDescription, bIsbn, bAsin, bLanguage, bCoverPath string
		var bDuration float64
		var bNarrators, bAudioFiles, bEbookFile, bChapters, bTags, bGenres, bLockedFields []byte
		var bExplicit, bAbridged int

		err = db.QueryRow(`
			SELECT title, titleIgnorePrefix, subtitle, publishedYear, publishedDate, publisher, description, isbn, asin, language, explicit, abridged, coverPath, duration, narrators, audioFiles, ebookFile, chapters, tags, genres, lockedFields
			FROM books WHERE id = ?
		`, mediaID).Scan(
			&bTitle, &bTitleIgnorePrefix, &bSubtitle, &bPublishedYear, &bPublishedDate, &bPublisher, &bDescription, &bIsbn, &bAsin, &bLanguage, &bExplicit, &bAbridged, &bCoverPath, &bDuration, &bNarrators, &bAudioFiles, &bEbookFile, &bChapters, &bTags, &bGenres, &bLockedFields,
		)
		if err == nil {
			var tags []string
			_ = json.Unmarshal(bTags, &tags)
			var genres []string
			_ = json.Unmarshal(bGenres, &genres)
			var audioFiles []interface{}
			_ = json.Unmarshal(bAudioFiles, &audioFiles)
			var chapters []interface{}
			_ = json.Unmarshal(bChapters, &chapters)
			var narratorNames []string
			_ = json.Unmarshal(bNarrators, &narratorNames)
			var lockedFields []string
			if len(bLockedFields) > 0 {
				_ = json.Unmarshal(bLockedFields, &lockedFields)
			}
			if lockedFields == nil {
				lockedFields = []string{}
			}

			var authorNames []string
			rows, err2 := db.Query("SELECT name FROM authors WHERE id IN (SELECT authorId FROM bookAuthors WHERE bookId = ?)", mediaID)
			if err2 == nil {
				defer rows.Close()
				for rows.Next() {
					var name string
					if err := rows.Scan(&name); err == nil {
						authorNames = append(authorNames, name)
					}
				}
			}

			var seriesList []*BookSeriesMinifiedJSON
			var seriesNames []string
			srows, err3 := db.Query("SELECT s.id, s.name, bs.sequence FROM series s JOIN bookSeries bs ON s.id = bs.seriesId WHERE bs.bookId = ?", mediaID)
			if err3 == nil {
				defer srows.Close()
				for srows.Next() {
					var sid, name string
					var sequence sql.NullString
					if err := srows.Scan(&sid, &name, &sequence); err == nil {
						var seqVal string
						if sequence.Valid {
							seqVal = sequence.String
						}
						seriesList = append(seriesList, &BookSeriesMinifiedJSON{
							ID:       sid,
							Name:     name,
							Sequence: seqVal,
						})
						if seqVal != "" {
							seriesNames = append(seriesNames, fmt.Sprintf("%s #%s", name, seqVal))
						} else {
							seriesNames = append(seriesNames, name)
						}
					}
				}
			}

			var firstSeq *string
			if len(seriesList) > 0 && seriesList[0].Sequence != "" {
				firstSeq = &seriesList[0].Sequence
			}

			var ebookFormat *string
			if len(bEbookFile) > 0 {
				var eb struct {
					EbookFormat string `json:"ebookFormat"`
				}
				if jsonUnmarshalSafe(bEbookFile, &eb) && eb.EbookFormat != "" {
					ebookFormat = &eb.EbookFormat
				}
			}

			authorName := strings.Join(authorNames, ", ")
			seriesName := strings.Join(seriesNames, ", ")
			narratorName := strings.Join(narratorNames, ", ")

			bookMin := &BookMinifiedJSON{
				ID:            mediaID,
				CoverPath:     nullIfEmpty(bCoverPath),
				Tags:          tags,
				NumTracks:     len(audioFiles),
				NumAudioFiles: len(audioFiles),
				NumChapters:   len(chapters),
				Duration:      bDuration,
				Size:          size,
				EbookFormat:   ebookFormat,
				AudioFiles:    audioFiles,
				Metadata: &BookMetadataMinified{
					Title:             bTitle,
					TitleIgnorePrefix: bTitleIgnorePrefix,
					Subtitle:          nullIfEmpty(bSubtitle),
					AuthorName:        authorName,
					AuthorNameLF:      utils.NameToLastFirst(authorName),
					NarratorName:      narratorName,
					SeriesName:        seriesName,
					SeriesSequence:    firstSeq,
					Series:            seriesList,
					Genres:            genres,
					PublishedYear:     nullIfEmpty(bPublishedYear),
					PublishedDate:     nullIfEmpty(bPublishedDate),
					Publisher:         nullIfEmpty(bPublisher),
					Description:       nullIfEmpty(bDescription),
					Isbn:              nullIfEmpty(bIsbn),
					Asin:              nullIfEmpty(bAsin),
					Language:          nullIfEmpty(bLanguage),
					Explicit:          bExplicit != 0,
					Abridged:          bAbridged != 0,
					LockedFields:      lockedFields,
				},
			}
			li.Media = bookMin
		}
	} else if mediaType == "podcast" {
		var pTitle, pTitleIgnorePrefix, pAuthor, pReleaseDate, pFeedURL, pImageURL, pDescription, pItunesPageURL, pItunesID, pItunesArtistID, pLanguage, pPodcastType, pCoverPath string
		var pExplicit, pAutoDownloadEpisodes, pMaxEpisodesToKeep, pMaxNewEpisodesToDownload, pNumEpisodes int
		var pTags, pGenres, pLockedFields []byte

		err = db.QueryRow(`
			SELECT title, titleIgnorePrefix, author, releaseDate, feedURL, imageURL, description, itunesPageURL, itunesId, itunesArtistId, language, podcastType, explicit, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload, coverPath, tags, genres, numEpisodes, lockedFields
			FROM podcasts WHERE id = ?
		`, mediaID).Scan(
			&pTitle, &pTitleIgnorePrefix, &pAuthor, &pReleaseDate, &pFeedURL, &pImageURL, &pDescription, &pItunesPageURL, &pItunesID, &pItunesArtistID, &pLanguage, &pPodcastType, &pExplicit, &pAutoDownloadEpisodes, &pMaxEpisodesToKeep, &pMaxNewEpisodesToDownload, &pCoverPath, &pTags, &pGenres, &pNumEpisodes, &pLockedFields,
		)
		if err == nil {
			var tags []string
			_ = json.Unmarshal(pTags, &tags)
			var genres []string
			_ = json.Unmarshal(pGenres, &genres)
			var lockedFields []string
			if len(pLockedFields) > 0 {
				_ = json.Unmarshal(pLockedFields, &lockedFields)
			}
			if lockedFields == nil {
				lockedFields = []string{}
			}

			var episodes []interface{}
			erows, err4 := db.Query("SELECT id, title, audioFile, pubDate, description, season, episode, episodeType, enclosureURL, publishedAt FROM podcastEpisodes WHERE podcastId = ?", mediaID)
			if err4 == nil {
				defer erows.Close()
				for erows.Next() {
					var epId, epTitle string
					var epAudioFile []byte
					var epPubDate, epDesc, epSeason, epEpisode, epEpType, epEncURL, epPubAt sql.NullString
					if err := erows.Scan(&epId, &epTitle, &epAudioFile, &epPubDate, &epDesc, &epSeason, &epEpisode, &epEpType, &epEncURL, &epPubAt); err == nil {
						var audioFile interface{}
						if len(epAudioFile) > 0 {
							_ = json.Unmarshal(epAudioFile, &audioFile)
						}
						epMap := map[string]interface{}{
							"id":        epId,
							"title":     epTitle,
							"audioFile": audioFile,
						}
						if epPubDate.Valid {
							epMap["pubDate"] = epPubDate.String
						}
						if epDesc.Valid {
							epMap["description"] = epDesc.String
						}
						if epSeason.Valid {
							epMap["season"] = epSeason.String
						}
						if epEpisode.Valid {
							epMap["episode"] = epEpisode.String
						}
						if epEpType.Valid {
							epMap["episodeType"] = epEpType.String
						}
						if epEncURL.Valid {
							epMap["enclosureUrl"] = epEncURL.String
						}
						if epPubAt.Valid {
							epMap["publishedAt"] = epPubAt.String
						}
						episodes = append(episodes, epMap)
					}
				}
			}

			podcastMin := &PodcastMinifiedJSON{
				ID:                       mediaID,
				CoverPath:                nullIfEmpty(pCoverPath),
				Tags:                     tags,
				NumEpisodes:              pNumEpisodes,
				AutoDownloadEpisodes:     pAutoDownloadEpisodes != 0,
				MaxEpisodesToKeep:        pMaxEpisodesToKeep,
				MaxNewEpisodesToDownload: pMaxNewEpisodesToDownload,
				Size:                     size,
				Episodes:                 episodes,
				Metadata: &PodcastMetadataMin{
					Title:             pTitle,
					TitleIgnorePrefix: pTitleIgnorePrefix,
					Author:            nullIfEmpty(pAuthor),
					Description:       nullIfEmpty(pDescription),
					ReleaseDate:       nullIfEmpty(pReleaseDate),
					Genres:            genres,
					FeedURL:           nullIfEmpty(pFeedURL),
					ImageURL:          nullIfEmpty(pImageURL),
					ItunesPageURL:     nullIfEmpty(pItunesPageURL),
					ItunesID:          nullIfEmpty(pItunesID),
					ItunesArtistID:    nullIfEmpty(pItunesArtistID),
					Explicit:          pExplicit != 0,
					Language:          nullIfEmpty(pLanguage),
					Type:              nullIfEmpty(pPodcastType),
					LockedFields:      lockedFields,
				},
			}
			li.Media = podcastMin
		}
	}

	return &li, nil
}

func GetFilteredLibraryItems(db *sql.DB, options GetFilteredLibraryItemsOptions) ([]*LibraryItemMinifiedJSON, int, error) {
	sortingIgnorePrefix := GetSortingIgnorePrefix(db)

	var conds []string
	var args []interface{}

	conds = append(conds, "li.libraryId = ?")
	args = append(args, options.LibraryID)

	if len(options.Include) > 0 {
		var placeholders []string
		for _, inc := range options.Include {
			placeholders = append(placeholders, "?")
			args = append(args, inc)
		}
		conds = append(conds, fmt.Sprintf("li.id IN (%s)", strings.Join(placeholders, ", ")))
	}

	var tableAlias string
	if options.MediaType == "book" {
		tableAlias = "b"
	} else {
		tableAlias = "p"
	}

	permCond, permArgs := getUserPermissionWhere(options.User, tableAlias)
	if permCond != "" {
		conds = append(conds, permCond)
		args = append(args, permArgs...)
	}

	filterCond, filterArgs := getFilterWhere(options.FilterBy, options.MediaType, tableAlias, "li", options.User.ID)
	if filterCond != "" {
		conds = append(conds, filterCond)
		args = append(args, filterArgs...)
	}

	if options.Search != "" {
		searchTerm := "%" + options.Search + "%"
		if options.MediaType == "book" {
			conds = append(conds, "(b.title LIKE ? OR li.authorNamesFirstLast LIKE ? OR b.subtitle LIKE ? OR b.description LIKE ?)")
			args = append(args, searchTerm, searchTerm, searchTerm, searchTerm)
		} else {
			conds = append(conds, "(p.title LIKE ? OR p.author LIKE ? OR p.description LIKE ?)")
			args = append(args, searchTerm, searchTerm, searchTerm)
		}
	}

	whereClause := "WHERE " + strings.Join(conds, " AND ")

	// Count query
	var countQuery string
	if options.MediaType == "book" {
		countQuery = fmt.Sprintf("SELECT COUNT(*) FROM libraryItems li JOIN books b ON li.mediaId = b.id AND li.mediaType = 'book' %s", whereClause)
	} else {
		countQuery = fmt.Sprintf("SELECT COUNT(*) FROM libraryItems li JOIN podcasts p ON li.mediaId = p.id AND li.mediaType = 'podcast' %s", whereClause)
	}

	var total int
	err := db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	orderClause := "ORDER BY " + getSortOrder(options.SortBy, options.SortDesc, sortingIgnorePrefix, options.MediaType, options.User.ID, &args)

	limitOffsetClause := ""
	if options.Limit > 0 {
		limitOffsetClause = fmt.Sprintf("LIMIT %d OFFSET %d", options.Limit, options.Page*options.Limit)
	}

	// Select query
	var selectQuery string
	if options.MediaType == "book" {
		selectQuery = fmt.Sprintf(`
			SELECT 
				li.id, li.ino, li.path, li.relPath, li.isFile, li.mtime, li.ctime, li.birthtime, li.createdAt, li.updatedAt, li.isMissing, li.isInvalid, li.mediaType, li.mediaId, li.size, li.libraryFolderId, li.authorNamesFirstLast, li.authorNamesLastFirst,
				b.id, b.title, b.titleIgnorePrefix, b.subtitle, b.publishedYear, b.publishedDate, b.publisher, b.description, b.isbn, b.asin, b.language, b.explicit, b.abridged, b.coverPath, b.duration, b.narrators, b.audioFiles, b.ebookFile, b.chapters, b.tags, b.genres
			FROM libraryItems li
			JOIN books b ON li.mediaId = b.id AND li.mediaType = 'book'
			%s
			%s
			%s
		`, whereClause, orderClause, limitOffsetClause)
	} else {
		selectQuery = fmt.Sprintf(`
			SELECT 
				li.id, li.ino, li.path, li.relPath, li.isFile, li.mtime, li.ctime, li.birthtime, li.createdAt, li.updatedAt, li.isMissing, li.isInvalid, li.mediaType, li.mediaId, li.size, li.libraryFolderId,
				p.id, p.title, p.titleIgnorePrefix, p.author, p.releaseDate, p.feedURL, p.imageURL, p.description, p.itunesPageURL, p.itunesId, p.itunesArtistId, p.language, p.podcastType, p.explicit, p.autoDownloadEpisodes, p.autoDownloadSchedule, p.lastEpisodeCheck, p.maxEpisodesToKeep, p.maxNewEpisodesToDownload, p.coverPath, p.tags, p.genres, p.numEpisodes
			FROM libraryItems li
			JOIN podcasts p ON li.mediaId = p.id AND li.mediaType = 'podcast'
			%s
			%s
			%s
		`, whereClause, orderClause, limitOffsetClause)
	}

	rows, err := db.Query(selectQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []*LibraryItemMinifiedJSON = []*LibraryItemMinifiedJSON{}
	var bookIDs []string
	bookMap := make(map[string]*BookMinifiedJSON)

	for rows.Next() {
		var id string
		var ino, path, relPath, mediaType, mediaID, libraryFolderID sql.NullString
		var isFileVal, isMissingVal, isInvalidVal sql.NullInt64
		var mtimeStr, ctimeStr, birthtimeStr, createdAtStr, updatedAtStr sql.NullString
		var size sql.NullInt64

		if options.MediaType == "book" {
			var authorNamesFirstLast, authorNamesLastFirst sql.NullString
			var bID, bTitle string
			var bTitleIgnorePrefix sql.NullString
			var bSubtitle, bPublishedYear, bPublishedDate, bPublisher, bDescription, bIsbn, bAsin, bLanguage, bCoverPath sql.NullString
			var bExplicit, bAbridged sql.NullInt64
			var bDuration sql.NullFloat64
			var bNarrators, bAudioFiles, bEbookFile, bChapters, bTags, bGenres []byte

			err = rows.Scan(
				&id, &ino, &path, &relPath, &isFileVal, &mtimeStr, &ctimeStr, &birthtimeStr, &createdAtStr, &updatedAtStr, &isMissingVal, &isInvalidVal, &mediaType, &mediaID, &size, &libraryFolderID, &authorNamesFirstLast, &authorNamesLastFirst,
				&bID, &bTitle, &bTitleIgnorePrefix, &bSubtitle, &bPublishedYear, &bPublishedDate, &bPublisher, &bDescription, &bIsbn, &bAsin, &bLanguage, &bExplicit, &bAbridged, &bCoverPath, &bDuration, &bNarrators, &bAudioFiles, &bEbookFile, &bChapters, &bTags, &bGenres,
			)
			if err != nil {
				return nil, 0, err
			}

			// Parse book sub-objects
			var tags []string
			if len(bTags) > 0 {
				json.Unmarshal(bTags, &tags)
			}
			var genres []string
			if len(bGenres) > 0 {
				json.Unmarshal(bGenres, &genres)
			}
			var audioFiles []struct {
				Exclude  bool `json:"exclude"`
				Metadata struct {
					Size int64 `json:"size"`
				} `json:"metadata"`
			}
			if len(bAudioFiles) > 0 {
				json.Unmarshal(bAudioFiles, &audioFiles)
			}
			var ebook struct {
				EbookFormat string `json:"ebookFormat"`
				Metadata    struct {
					Size int64 `json:"size"`
				} `json:"metadata"`
			}
			if len(bEbookFile) > 0 {
				json.Unmarshal(bEbookFile, &ebook)
			}
			var chapters []interface{}
			if len(bChapters) > 0 {
				json.Unmarshal(bChapters, &chapters)
			}

			numTracks := 0
			for _, af := range audioFiles {
				if !af.Exclude {
					numTracks++
				}
			}

			var ebookFormat *string
			if ebook.EbookFormat != "" {
				val := ebook.EbookFormat
				ebookFormat = &val
			}

			var cover *string
			if bCoverPath.Valid && bCoverPath.String != "" {
				cover = &bCoverPath.String
			}

			var subtitleVal *string
			if bSubtitle.Valid {
				subtitleVal = &bSubtitle.String
			}
			var publishedYearVal *string
			if bPublishedYear.Valid {
				publishedYearVal = &bPublishedYear.String
			}
			var publishedDateVal *string
			if bPublishedDate.Valid {
				publishedDateVal = &bPublishedDate.String
			}
			var publisherVal *string
			if bPublisher.Valid {
				publisherVal = &bPublisher.String
			}
			var descriptionVal *string
			if bDescription.Valid {
				descriptionVal = &bDescription.String
			}
			var isbnVal *string
			if bIsbn.Valid {
				isbnVal = &bIsbn.String
			}
			var asinVal *string
			if bAsin.Valid {
				asinVal = &bAsin.String
			}
			var languageVal *string
			if bLanguage.Valid {
				languageVal = &bLanguage.String
			}

			var calculatedSize int64
			for _, af := range audioFiles {
				calculatedSize += af.Metadata.Size
			}
			calculatedSize += ebook.Metadata.Size
			if calculatedSize == 0 {
				calculatedSize = size.Int64
			}

			bookMin := &BookMinifiedJSON{
				ID:            bID,
				CoverPath:     cover,
				Tags:          tags,
				NumTracks:     numTracks,
				NumAudioFiles: len(audioFiles),
				NumChapters:   len(chapters),
				Duration:      bDuration.Float64,
				Size:          calculatedSize,
				EbookFormat:   ebookFormat,
				Metadata: &BookMetadataMinified{
					Title:             bTitle,
					TitleIgnorePrefix: bTitleIgnorePrefix.String,
					Subtitle:          subtitleVal,
					AuthorName:        authorNamesFirstLast.String,
					AuthorNameLF:      authorNamesLastFirst.String,
					NarratorName:      jsonArrayToCommaString(bNarrators),
					SeriesName:        "", // Filled later
					Genres:            genres,
					PublishedYear:     publishedYearVal,
					PublishedDate:     publishedDateVal,
					Publisher:         publisherVal,
					Description:       descriptionVal,
					Isbn:              isbnVal,
					Asin:              asinVal,
					Language:          languageVal,
					Explicit:          bExplicit.Valid && bExplicit.Int64 != 0,
					Abridged:          bAbridged.Valid && bAbridged.Int64 != 0,
				},
			}

			bookIDs = append(bookIDs, bID)
			bookMap[bID] = bookMin

			liMin := &LibraryItemMinifiedJSON{
				ID:          id,
				Ino:         ino.String,
				LibraryID:   options.LibraryID,
				FolderID:    libraryFolderID.String,
				Path:        path.String,
				RelPath:     relPath.String,
				IsFile:      isFileVal.Valid && isFileVal.Int64 != 0,
				MtimeMs:     parseEpochMillis(mtimeStr.String),
				CtimeMs:     parseEpochMillis(ctimeStr.String),
				BirthtimeMs: parseEpochMillis(birthtimeStr.String),
				AddedAt:     parseEpochMillis(createdAtStr.String),
				UpdatedAt:   parseEpochMillis(updatedAtStr.String),
				IsMissing:   isMissingVal.Valid && isMissingVal.Int64 != 0,
				IsInvalid:   isInvalidVal.Valid && isInvalidVal.Int64 != 0,
				MediaType:   mediaType.String,
				Media:       bookMin,
				NumFiles:    len(audioFiles) + len(chapters), // fallback files count
				Size:        calculatedSize,
			}

			results = append(results, liMin)
		} else {
			var pID, pTitle string
			var pTitleIgnorePrefix sql.NullString
			var pAuthor, pReleaseDate, pFeedURL, pImageURL, pDescription, pItunesPageURL, pItunesID, pItunesArtistID, pLanguage, pPodcastType, pAutoDownloadSchedule, pCoverPath sql.NullString
			var pExplicit, pAutoDownloadEpisodes, pMaxEpisodesToKeep, pMaxNewEpisodesToDownload, pNumEpisodes sql.NullInt64
			var pLastEpisodeCheck sql.NullString
			var pTags, pGenres []byte

			err = rows.Scan(
				&id, &ino, &path, &relPath, &isFileVal, &mtimeStr, &ctimeStr, &birthtimeStr, &createdAtStr, &updatedAtStr, &isMissingVal, &isInvalidVal, &mediaType, &mediaID, &size, &libraryFolderID,
				&pID, &pTitle, &pTitleIgnorePrefix, &pAuthor, &pReleaseDate, &pFeedURL, &pImageURL, &pDescription, &pItunesPageURL, &pItunesID, &pItunesArtistID, &pLanguage, &pPodcastType, &pExplicit, &pAutoDownloadEpisodes, &pAutoDownloadSchedule, &pLastEpisodeCheck, &pMaxEpisodesToKeep, &pMaxNewEpisodesToDownload, &pCoverPath, &pTags, &pGenres, &pNumEpisodes,
			)
			if err != nil {
				return nil, 0, err
			}

			var tags []string
			if len(pTags) > 0 {
				json.Unmarshal(pTags, &tags)
			}
			var genres []string
			if len(pGenres) > 0 {
				json.Unmarshal(pGenres, &genres)
			}

			var cover *string
			if pCoverPath.Valid && pCoverPath.String != "" {
				cover = &pCoverPath.String
			}

			var authorVal *string
			if pAuthor.Valid {
				authorVal = &pAuthor.String
			}
			var descriptionVal *string
			if pDescription.Valid {
				descriptionVal = &pDescription.String
			}
			var releaseDateVal *string
			if pReleaseDate.Valid {
				releaseDateVal = &pReleaseDate.String
			}
			var feedURLVal *string
			if pFeedURL.Valid {
				feedURLVal = &pFeedURL.String
			}
			var imageURLVal *string
			if pImageURL.Valid {
				imageURLVal = &pImageURL.String
			}
			var itunesPageURLVal *string
			if pItunesPageURL.Valid {
				itunesPageURLVal = &pItunesPageURL.String
			}
			var itunesIDVal *string
			if pItunesID.Valid {
				itunesIDVal = &pItunesID.String
			}
			var itunesArtistIDVal *string
			if pItunesArtistID.Valid {
				itunesArtistIDVal = &pItunesArtistID.String
			}
			var languageVal *string
			if pLanguage.Valid {
				languageVal = &pLanguage.String
			}
			var podcastTypeVal *string
			if pPodcastType.Valid {
				podcastTypeVal = &pPodcastType.String
			}
			var autoDownloadScheduleVal *string
			if pAutoDownloadSchedule.Valid {
				autoDownloadScheduleVal = &pAutoDownloadSchedule.String
			}

			var lastEpisodeCheckVal *int64
			if pLastEpisodeCheck.Valid && pLastEpisodeCheck.String != "" {
				t, err := ParseSQLiteTime(pLastEpisodeCheck.String)
				if err == nil {
					val := t.UnixNano() / int64(time.Millisecond)
					lastEpisodeCheckVal = &val
				}
			}

			podcastMin := &PodcastMinifiedJSON{
				ID:                       pID,
				CoverPath:                cover,
				Tags:                     tags,
				NumEpisodes:              int(pNumEpisodes.Int64),
				AutoDownloadEpisodes:     pAutoDownloadEpisodes.Valid && pAutoDownloadEpisodes.Int64 != 0,
				AutoDownloadSchedule:     autoDownloadScheduleVal,
				LastEpisodeCheck:         lastEpisodeCheckVal,
				MaxEpisodesToKeep:        int(pMaxEpisodesToKeep.Int64),
				MaxNewEpisodesToDownload: int(pMaxNewEpisodesToDownload.Int64),
				Size:                     size.Int64,
				Metadata: &PodcastMetadataMin{
					Title:             pTitle,
					TitleIgnorePrefix: pTitleIgnorePrefix.String,
					Author:            authorVal,
					Description:       descriptionVal,
					ReleaseDate:       releaseDateVal,
					Genres:            genres,
					FeedURL:           feedURLVal,
					ImageURL:          imageURLVal,
					ItunesPageURL:     itunesPageURLVal,
					ItunesID:          itunesIDVal,
					ItunesArtistID:    itunesArtistIDVal,
					Explicit:          pExplicit.Valid && pExplicit.Int64 != 0,
					Language:          languageVal,
					Type:              podcastTypeVal,
				},
			}

			liMin := &LibraryItemMinifiedJSON{
				ID:          id,
				Ino:         ino.String,
				LibraryID:   options.LibraryID,
				FolderID:    libraryFolderID.String,
				Path:        path.String,
				RelPath:     relPath.String,
				IsFile:      isFileVal.Valid && isFileVal.Int64 != 0,
				MtimeMs:     parseEpochMillis(mtimeStr.String),
				CtimeMs:     parseEpochMillis(ctimeStr.String),
				BirthtimeMs: parseEpochMillis(birthtimeStr.String),
				AddedAt:     parseEpochMillis(createdAtStr.String),
				UpdatedAt:   parseEpochMillis(updatedAtStr.String),
				IsMissing:   isMissingVal.Valid && isMissingVal.Int64 != 0,
				IsInvalid:   isInvalidVal.Valid && isInvalidVal.Int64 != 0,
				MediaType:   mediaType.String,
				Media:       podcastMin,
				NumFiles:    int(pNumEpisodes.Int64),
				Size:        size.Int64,
			}

			results = append(results, liMin)
		}
	}

	// Fetch series for the selected books to populate seriesName
	if len(bookIDs) > 0 {
		placeholders := make([]string, len(bookIDs))
		queryArgs := make([]interface{}, len(bookIDs))
		for i, id := range bookIDs {
			placeholders[i] = "?"
			queryArgs[i] = id
		}

		seriesQuery := fmt.Sprintf(`
			SELECT bs.bookId, s.id, s.name, bs.sequence
			FROM bookSeries bs
			JOIN series s ON bs.seriesId = s.id
			WHERE bs.bookId IN (%s)
			ORDER BY CAST(bs.sequence AS FLOAT) ASC NULLS LAST
		`, strings.Join(placeholders, ","))

		sRows, err := db.Query(seriesQuery, queryArgs...)
		if err == nil {
			defer sRows.Close()

			bookSeriesMap := make(map[string][]*BookSeriesMinifiedJSON)
			for sRows.Next() {
				var bookID, seriesID, seriesName string
				var sequence sql.NullString
				if err := sRows.Scan(&bookID, &seriesID, &seriesName, &sequence); err == nil {
					var seqVal string
					if sequence.Valid {
						seqVal = sequence.String
					}
					bookSeriesMap[bookID] = append(bookSeriesMap[bookID], &BookSeriesMinifiedJSON{
						ID:       seriesID,
						Name:     seriesName,
						Sequence: seqVal,
					})
				}
			}

			for bID, bookMin := range bookMap {
				if sList, ok := bookSeriesMap[bID]; ok {
					bookMin.Metadata.Series = sList

					var nameSeqs []string
					for _, s := range sList {
						if s.Sequence != "" {
							nameSeqs = append(nameSeqs, fmt.Sprintf("%s #%s", s.Name, s.Sequence))
						} else {
							nameSeqs = append(nameSeqs, s.Name)
						}
					}
					bookMin.Metadata.SeriesName = strings.Join(nameSeqs, ", ")

					if len(sList) > 0 && sList[0].Sequence != "" {
						seq := sList[0].Sequence
						bookMin.Metadata.SeriesSequence = &seq
					}
				}
			}
		}
	}

	return results, total, nil
}
