package db

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"audiobookshelf/internal/core"
	"audiobookshelf/internal/utils"
	watcher "audiobookshelf/internal/watcher"
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

type LibraryFolderJSON struct {
	ID        string `json:"id"`
	FullPath  string `json:"fullPath"`
	LibraryID string `json:"libraryId"`
	AddedAt   int64  `json:"addedAt"`
}

type LibraryJSON struct {
	ID              string               `json:"id"`
	Name            string               `json:"name"`
	Folders         []*LibraryFolderJSON `json:"folders"`
	DisplayOrder    int                  `json:"displayOrder"`
	Icon            string               `json:"icon"`
	MediaType       string               `json:"mediaType"`
	Provider        string               `json:"provider"`
	Settings        json.RawMessage      `json:"settings"`
	LastScan        *int64               `json:"lastScan"`
	LastScanVersion string               `json:"lastScanVersion"`
	CreatedAt       int64                `json:"createdAt"`
	LastUpdate      int64                `json:"lastUpdate"`
	Stats           *LibraryStats        `json:"stats,omitempty"`
}

type LibraryStats struct {
	TotalSize     int64   `json:"totalSize"`
	TotalDuration float64 `json:"totalDuration"`
	NumAudioFiles int     `json:"numAudioFiles"`
	TotalItems    int     `json:"totalItems"`
}

// GetLibraries returns all libraries in the database
func GetLibraries(db *sql.DB) ([]*LibraryJSON, error) {
	rows, err := db.Query("SELECT id, name, displayOrder, icon, mediaType, provider, lastScan, lastScanVersion, settings, createdAt, updatedAt FROM libraries ORDER BY displayOrder ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var libraries []*LibraryJSON
	for rows.Next() {
		var id, name, mediaType, createdAtStr, updatedAtStr string
		var icon, provider, lastScanVersion sql.NullString
		var displayOrder int
		var lastScanStr sql.NullString
		var settingsBytes []byte

		err = rows.Scan(&id, &name, &displayOrder, &icon, &mediaType, &provider, &lastScanStr, &lastScanVersion, &settingsBytes, &createdAtStr, &updatedAtStr)
		if err != nil {
			return nil, err
		}

		var lastScan *int64
		if lastScanStr.Valid && lastScanStr.String != "" {
			t, err := ParseSQLiteTime(lastScanStr.String)
			if err == nil {
				val := t.UnixNano() / int64(time.Millisecond)
				lastScan = &val
			}
		}

		createdAtTime, _ := ParseSQLiteTime(createdAtStr)
		updatedAtTime, _ := ParseSQLiteTime(updatedAtStr)

		var iconVal string
		if icon.Valid {
			iconVal = icon.String
		}
		var providerVal string
		if provider.Valid {
			providerVal = provider.String
		}
		var lastScanVersionVal string
		if lastScanVersion.Valid {
			lastScanVersionVal = lastScanVersion.String
		}

		lib := &LibraryJSON{
			ID:              id,
			Name:            name,
			DisplayOrder:    displayOrder,
			Icon:            iconVal,
			MediaType:       mediaType,
			Provider:        providerVal,
			LastScan:        lastScan,
			LastScanVersion: lastScanVersionVal,
			Settings:        json.RawMessage(settingsBytes),
			CreatedAt:       createdAtTime.UnixNano() / int64(time.Millisecond),
			LastUpdate:      updatedAtTime.UnixNano() / int64(time.Millisecond),
			Folders:         []*LibraryFolderJSON{},
		}
		if len(lib.Settings) == 0 {
			lib.Settings = json.RawMessage(`{}`)
		}

		libraries = append(libraries, lib)
	}

	// Fetch all library folders and attach them
	folderRows, err := db.Query("SELECT id, path, libraryId, createdAt FROM libraryFolders")
	if err != nil {
		return nil, err
	}
	defer folderRows.Close()

	foldersByLibrary := make(map[string][]*LibraryFolderJSON)
	for folderRows.Next() {
		var id, path, libraryID, createdAtStr string
		err = folderRows.Scan(&id, &path, &libraryID, &createdAtStr)
		if err != nil {
			return nil, err
		}

		createdAtTime, _ := ParseSQLiteTime(createdAtStr)
		folder := &LibraryFolderJSON{
			ID:        id,
			FullPath:  path,
			LibraryID: libraryID,
			AddedAt:   createdAtTime.UnixNano() / int64(time.Millisecond),
		}
		foldersByLibrary[libraryID] = append(foldersByLibrary[libraryID], folder)
	}

	for _, lib := range libraries {
		if folders, ok := foldersByLibrary[lib.ID]; ok {
			lib.Folders = folders
		}
	}

	return libraries, nil
}

// GetLibraryByID fetches a single library by ID
func GetLibraryByID(db *sql.DB, libraryID string) (*LibraryJSON, error) {
	var id, name, mediaType, createdAtStr, updatedAtStr string
	var icon, provider, lastScanVersion sql.NullString
	var displayOrder int
	var lastScanStr sql.NullString
	var settingsBytes []byte

	err := db.QueryRow("SELECT id, name, displayOrder, icon, mediaType, provider, lastScan, lastScanVersion, settings, createdAt, updatedAt FROM libraries WHERE id = ?", libraryID).
		Scan(&id, &name, &displayOrder, &icon, &mediaType, &provider, &lastScanStr, &lastScanVersion, &settingsBytes, &createdAtStr, &updatedAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var lastScan *int64
	if lastScanStr.Valid && lastScanStr.String != "" {
		t, err := ParseSQLiteTime(lastScanStr.String)
		if err == nil {
			val := t.UnixNano() / int64(time.Millisecond)
			lastScan = &val
		}
	}

	createdAtTime, _ := ParseSQLiteTime(createdAtStr)
	updatedAtTime, _ := ParseSQLiteTime(updatedAtStr)

	var iconVal string
	if icon.Valid {
		iconVal = icon.String
	}
	var providerVal string
	if provider.Valid {
		providerVal = provider.String
	}
	var lastScanVersionVal string
	if lastScanVersion.Valid {
		lastScanVersionVal = lastScanVersion.String
	}

	lib := &LibraryJSON{
		ID:              id,
		Name:            name,
		DisplayOrder:    displayOrder,
		Icon:            iconVal,
		MediaType:       mediaType,
		Provider:        providerVal,
		LastScan:        lastScan,
		LastScanVersion: lastScanVersionVal,
		Settings:        json.RawMessage(settingsBytes),
		CreatedAt:       createdAtTime.UnixNano() / int64(time.Millisecond),
		LastUpdate:      updatedAtTime.UnixNano() / int64(time.Millisecond),
		Folders:         []*LibraryFolderJSON{},
	}
	if len(lib.Settings) == 0 {
		lib.Settings = json.RawMessage(`{}`)
	}

	// Fetch folders
	rows, err := db.Query("SELECT id, path, libraryId, createdAt FROM libraryFolders WHERE libraryId = ?", libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var folderID, path, folderLibID, folderCreatedAtStr string
		err = rows.Scan(&folderID, &path, &folderLibID, &folderCreatedAtStr)
		if err != nil {
			return nil, err
		}
		fCreatedAt, _ := ParseSQLiteTime(folderCreatedAtStr)
		lib.Folders = append(lib.Folders, &LibraryFolderJSON{
			ID:        folderID,
			FullPath:  path,
			LibraryID: folderLibID,
			AddedAt:   fCreatedAt.UnixNano() / int64(time.Millisecond),
		})
	}

	return lib, nil
}

func GetBookLibraryStats(db *sql.DB, libraryID string) (*LibraryStats, error) {
	var stats LibraryStats
	query := `
		SELECT 
			COALESCE(SUM(li.size), 0) AS totalSize, 
			COALESCE(SUM(b.duration), 0) AS totalDuration, 
			COALESCE(SUM(json_array_length(b.audioFiles)), 0) AS numAudioFiles, 
			COUNT(*) AS totalItems 
		FROM libraryItems li
		JOIN books b ON b.id = li.mediaId AND li.mediaType = 'book'
		WHERE li.libraryId = ?
	`
	err := db.QueryRow(query, libraryID).Scan(&stats.TotalSize, &stats.TotalDuration, &stats.NumAudioFiles, &stats.TotalItems)
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

func GetPodcastLibraryStats(db *sql.DB, libraryID string) (*LibraryStats, error) {
	var totalSize int64
	err := db.QueryRow(`SELECT COALESCE(SUM(li.size), 0) FROM libraryItems li WHERE li.mediaType = "podcast" AND li.libraryId = ?`, libraryID).Scan(&totalSize)
	if err != nil {
		return nil, err
	}

	var stats LibraryStats
	stats.TotalSize = totalSize

	query := `
		SELECT 
			COALESCE(SUM(json_extract(pe.audioFile, '$.duration')), 0) AS totalDuration, 
			COUNT(DISTINCT li.id) AS totalItems, 
			COUNT(pe.id) AS numAudioFiles 
		FROM libraryItems li
		JOIN podcasts p ON p.id = li.mediaId AND li.mediaType = 'podcast'
		LEFT JOIN podcastEpisodes pe ON pe.podcastId = p.id 
		WHERE li.libraryId = ?
	`
	err = db.QueryRow(query, libraryID).Scan(&stats.TotalDuration, &stats.TotalItems, &stats.NumAudioFiles)
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

func decodeFilterValue(s string) string {
	decoded, err := url.QueryUnescape(s)
	if err != nil {
		decoded = s
	}
	data, err := base64.StdEncoding.DecodeString(decoded)
	if err != nil {
		return decoded
	}
	return string(data)
}

func getUserPermissionWhere(user *core.UserSession, tableAlias string) (string, []interface{}) {
	var conds []string
	var args []interface{}

	// Explicit content restriction
	if !user.CanAccessExplicitContent {
		conds = append(conds, fmt.Sprintf("%s.explicit = 0", tableAlias))
	}

	// Tag restriction
	if !user.AccessAllTags && len(user.ItemTagsSelected) > 0 {
		placeholders := ""
		for i, tag := range user.ItemTagsSelected {
			if i > 0 {
				placeholders += ","
			}
			placeholders += "?"
			args = append(args, tag)
		}
		if user.SelectedTagsNotAccessible {
			conds = append(conds, fmt.Sprintf("(SELECT count(*) FROM json_each(%s.tags) WHERE json_valid(%s.tags) AND json_each.value IN (%s)) = 0", tableAlias, tableAlias, placeholders))
		} else {
			conds = append(conds, fmt.Sprintf("(SELECT count(*) FROM json_each(%s.tags) WHERE json_valid(%s.tags) AND json_each.value IN (%s)) >= 1", tableAlias, tableAlias, placeholders))
		}
	}

	if len(conds) == 0 {
		return "", nil
	}

	return strings.Join(conds, " AND "), args
}

func getFilterWhere(filterBy string, mediaType string, tableAlias string, liAlias string, userID string) (string, []interface{}) {
	if filterBy == "" {
		return "", nil
	}
	parts := strings.SplitN(filterBy, ".", 2)
	group := parts[0]
	var value string
	if len(parts) == 2 {
		value = decodeFilterValue(parts[1])
	}

	switch group {
	case "authors":
		if mediaType == "book" {
			return fmt.Sprintf("%s.id IN (SELECT bookId FROM bookAuthors WHERE authorId = ?)", tableAlias), []interface{}{value}
		}
	case "series":
		if mediaType == "book" {
			if value == "no-series" {
				return fmt.Sprintf("NOT EXISTS (SELECT 1 FROM bookSeries bs WHERE bs.bookId = %s.id)", tableAlias), nil
			}
			return fmt.Sprintf("%s.id IN (SELECT bookId FROM bookSeries WHERE seriesId = ?)", tableAlias), []interface{}{value}
		}
	case "genres":
		return fmt.Sprintf("EXISTS (SELECT 1 FROM json_each(%s.genres) WHERE json_valid(%s.genres) AND json_each.value = ?)", tableAlias, tableAlias), []interface{}{value}
	case "tags":
		return fmt.Sprintf("EXISTS (SELECT 1 FROM json_each(%s.tags) WHERE json_valid(%s.tags) AND json_each.value = ?)", tableAlias, tableAlias), []interface{}{value}
	case "narrators":
		if mediaType == "book" {
			return fmt.Sprintf("EXISTS (SELECT 1 FROM json_each(%s.narrators) WHERE json_valid(%s.narrators) AND json_each.value = ?)", tableAlias, tableAlias), []interface{}{value}
		}
	case "languages":
		return fmt.Sprintf("%s.language = ?", tableAlias), []interface{}{value}
	case "publishers":
		if mediaType == "book" {
			return fmt.Sprintf("%s.publisher = ?", tableAlias), []interface{}{value}
		}
	case "progress":
		if value == "in-progress" {
			return fmt.Sprintf("EXISTS (SELECT 1 FROM mediaProgresses mp WHERE mp.userId = ? AND mp.mediaItemId = %s.id AND mp.isFinished = 0 AND mp.currentTime > 0)", tableAlias), []interface{}{userID}
		} else if value == "finished" {
			return fmt.Sprintf("EXISTS (SELECT 1 FROM mediaProgresses mp WHERE mp.userId = ? AND mp.mediaItemId = %s.id AND mp.isFinished = 1)", tableAlias), []interface{}{userID}
		} else if value == "not-started" {
			return fmt.Sprintf("NOT EXISTS (SELECT 1 FROM mediaProgresses mp WHERE mp.userId = ? AND mp.mediaItemId = %s.id AND (mp.isFinished = 1 OR mp.currentTime > 0))", tableAlias), []interface{}{userID}
		}
	case "missing":
		return fmt.Sprintf("(%s.isMissing = 1 OR %s.isInvalid = 1)", liAlias, liAlias), nil
	}
	return "", nil
}

func getSortOrder(sortBy string, sortDesc bool, sortingIgnorePrefix bool, mediaType string, userID string) string {
	dir := "ASC"
	if sortDesc {
		dir = "DESC"
	}

	titleCol := "li.title"
	if sortingIgnorePrefix {
		titleCol = "li.titleIgnorePrefix"
	}

	switch sortBy {
	case "addedAt":
		return fmt.Sprintf("li.createdAt %s", dir)
	case "size":
		return fmt.Sprintf("li.size %s", dir)
	case "birthtimeMs":
		return fmt.Sprintf("li.birthtime %s", dir)
	case "mtimeMs":
		return fmt.Sprintf("li.mtime %s", dir)
	case "media.duration":
		if mediaType == "book" {
			return fmt.Sprintf("b.duration %s", dir)
		}
	case "media.metadata.publishedYear":
		if mediaType == "book" {
			return fmt.Sprintf("CAST(b.publishedYear AS INTEGER) %s", dir)
		}
	case "media.metadata.authorNameLF":
		if mediaType == "book" {
			return fmt.Sprintf("li.authorNamesLastFirst COLLATE NOCASE %s, %s COLLATE NOCASE %s", dir, titleCol, dir)
		}
	case "media.metadata.authorName":
		if mediaType == "book" {
			return fmt.Sprintf("li.authorNamesFirstLast COLLATE NOCASE %s, %s COLLATE NOCASE %s", dir, titleCol, dir)
		}
	case "media.metadata.title":
		return fmt.Sprintf("%s COLLATE NOCASE %s", titleCol, dir)
	case "sequence":
		nullDir := "ASC NULLS LAST"
		if sortDesc {
			nullDir = "DESC NULLS FIRST"
		}
		if mediaType == "book" {
			return fmt.Sprintf("CAST((SELECT sequence FROM bookSeries bs WHERE bs.bookId = b.id LIMIT 1) AS FLOAT) %s", nullDir)
		}
	case "progress":
		nullDir := "ASC NULLS LAST"
		if sortDesc {
			nullDir = "DESC NULLS FIRST"
		}
		if mediaType == "book" {
			return fmt.Sprintf("(SELECT mp.updatedAt FROM mediaProgresses mp WHERE mp.mediaItemId = b.id AND mp.userId = '%s') %s", userID, nullDir)
		}
	case "media.metadata.author":
		if mediaType == "podcast" {
			return fmt.Sprintf("p.author COLLATE NOCASE %s", dir)
		}
	case "media.numTracks":
		if mediaType == "podcast" {
			return fmt.Sprintf("p.numEpisodes %s", dir)
		}
	case "random":
		return "random()"
	}

	return fmt.Sprintf("%s COLLATE NOCASE %s", titleCol, dir)
}

type GetFilteredLibraryItemsOptions struct {
	LibraryID      string
	User           *core.UserSession
	FilterBy       string
	SortBy         string
	SortDesc       bool
	Limit          int
	Page           int
	CollapseSeries bool
	Include        []string
	MediaType      string
	Minified       bool
	Search         string
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

			podcastMin := &PodcastMinifiedJSON{
				ID:                       mediaID,
				CoverPath:                nullIfEmpty(pCoverPath),
				Tags:                     tags,
				NumEpisodes:              pNumEpisodes,
				AutoDownloadEpisodes:     pAutoDownloadEpisodes != 0,
				MaxEpisodesToKeep:        pMaxEpisodesToKeep,
				MaxNewEpisodesToDownload: pMaxNewEpisodesToDownload,
				Size:                     size,
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

// jsonUnmarshalSafe unmarshals JSON safely, returning false on error.
func jsonUnmarshalSafe(data []byte, v interface{}) bool {
	return json.Unmarshal(data, v) == nil
}

func GetFilteredLibraryItems(db *sql.DB, options GetFilteredLibraryItemsOptions) ([]*LibraryItemMinifiedJSON, int, error) {
	sortingIgnorePrefix := GetSortingIgnorePrefix(db)

	var conds []string
	var args []interface{}

	conds = append(conds, "li.libraryId = ?")
	args = append(args, options.LibraryID)

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

	orderClause := "ORDER BY " + getSortOrder(options.SortBy, options.SortDesc, sortingIgnorePrefix, options.MediaType, options.User.ID)

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

// parseEpochMillis delegates to internal/db.
func parseEpochMillis(s string) int64 {
	return ParseEpochMillis(s)
}

// jsonArrayToCommaString delegates to internal/db.
func jsonArrayToCommaString(jsonBytes []byte) string {
	return JsonArrayToCommaString(jsonBytes)
}

type CreateFolderPayload struct {
	Path     string `json:"path"`
	FullPath string `json:"fullPath"`
}

type CreateLibraryPayload struct {
	Name      string                 `json:"name"`
	Folders   []CreateFolderPayload  `json:"folders"`
	MediaType string                 `json:"mediaType"`
	Icon      string                 `json:"icon"`
	Provider  string                 `json:"provider"`
	Settings  map[string]interface{} `json:"settings"`
}

type UpdateFolderPayload struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	FullPath string `json:"fullPath"`
}

type UpdateLibraryPayload struct {
	Name         *string                `json:"name"`
	Provider     *string                `json:"provider"`
	MediaType    *string                `json:"mediaType"`
	Icon         *string                `json:"icon"`
	DisplayOrder *int                   `json:"displayOrder"`
	Settings     map[string]interface{} `json:"settings"`
	Folders      []UpdateFolderPayload  `json:"folders"`
}

func getDefaultSettings(mediaType string) map[string]interface{} {
	if mediaType == "podcast" {
		return map[string]interface{}{
			"coverAspectRatio":              float64(1),
			"disableWatcher":                false,
			"autoScanCronExpression":        nil,
			"podcastSearchRegion":           "us",
			"markAsFinishedPercentComplete": nil,
			"markAsFinishedTimeRemaining":   float64(10),
		}
	}
	return map[string]interface{}{
		"coverAspectRatio":                   float64(1),
		"disableWatcher":                     false,
		"autoScanCronExpression":             nil,
		"skipMatchingMediaWithAsin":          false,
		"skipMatchingMediaWithIsbn":          false,
		"audiobooksOnly":                     false,
		"epubsAllowScriptedContent":          false,
		"hideSingleBookSeries":               false,
		"onlyShowLaterBooksInContinueSeries": false,
		"metadataPrecedence": []interface{}{
			"folderStructure", "audioMetatags", "nfoFile", "txtFiles", "opfFile", "absMetadata",
		},
		"markAsFinishedPercentComplete": nil,
		"markAsFinishedTimeRemaining":   float64(10),
	}
}

func mergeSettings(mediaType string, inputSettings map[string]interface{}) (map[string]interface{}, error) {
	settings := getDefaultSettings(mediaType)
	if inputSettings == nil {
		return settings, nil
	}

	for k, v := range inputSettings {
		if _, exists := settings[k]; !exists {
			continue
		}

		if v == nil {
			settings[k] = nil
			continue
		}

		if k == "metadataPrecedence" {
			arr, ok := v.([]interface{})
			if !ok {
				return nil, fmt.Errorf("settings \"metadataPrecedence\" must be an array")
			}
			for _, item := range arr {
				if _, ok := item.(string); !ok {
					return nil, fmt.Errorf("settings \"metadataPrecedence\" array elements must be strings")
				}
			}
			settings[k] = arr
		} else if k == "autoScanCronExpression" || k == "podcastSearchRegion" {
			if _, ok := v.(string); !ok {
				return nil, fmt.Errorf("settings \"%s\" must be a string", k)
			}
			settings[k] = v
		} else if k == "markAsFinishedPercentComplete" || k == "markAsFinishedTimeRemaining" {
			val, ok := v.(float64)
			if !ok {
				return nil, fmt.Errorf("setting \"%s\" must be a number", k)
			}
			if k == "markAsFinishedPercentComplete" {
				if val < 0 || val > 100 {
					return nil, fmt.Errorf("setting \"%s\" must be between 0 and 100", k)
				}
			} else if k == "markAsFinishedTimeRemaining" {
				if val < 0 {
					return nil, fmt.Errorf("setting \"%s\" must be greater than or equal to 0", k)
				}
			}
			settings[k] = val
		} else {
			switch settings[k].(type) {
			case bool:
				if _, ok := v.(bool); !ok {
					return nil, fmt.Errorf("setting \"%s\" must be of type bool", k)
				}
			case float64:
				if _, ok := v.(float64); !ok {
					return nil, fmt.Errorf("setting \"%s\" must be of type number", k)
				}
			case string:
				if _, ok := v.(string); !ok {
					return nil, fmt.Errorf("setting \"%s\" must be of type string", k)
				}
			}
			settings[k] = v
		}
	}
	return settings, nil
}

func TableExistsTx(tx *sql.Tx, tableName string) bool {
	var name string
	err := tx.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tableName).Scan(&name)
	return err == nil && name == tableName
}

func tableExistsTx(tx *sql.Tx, tableName string) bool {
	return TableExistsTx(tx, tableName)
}

func CreateLibrary(db *sql.DB, payload *CreateLibraryPayload) (*LibraryJSON, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if payload.MediaType == "" {
		payload.MediaType = "book"
	}
	if payload.Icon == "" {
		payload.Icon = "database"
	}
	if payload.Provider == "" {
		payload.Provider = "google"
	}

	merged, err := mergeSettings(payload.MediaType, payload.Settings)
	if err != nil {
		return nil, err
	}
	settingsBytes, err := json.Marshal(merged)
	if err != nil {
		return nil, err
	}

	var maxOrder int
	err = tx.QueryRow("SELECT COALESCE(MAX(displayOrder), 0) FROM libraries").Scan(&maxOrder)
	if err != nil {
		return nil, err
	}
	displayOrder := maxOrder + 1

	libraryID := uuid.New().String()
	nowStr := time.Now().UTC().Format("2006-01-02 15:04:05.000")

	_, err = tx.Exec(`
		INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, lastScan, lastScanVersion, settings, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?, ?)`,
		libraryID, payload.Name, displayOrder, payload.Icon, payload.MediaType, payload.Provider, settingsBytes, nowStr, nowStr)
	if err != nil {
		return nil, err
	}

	for _, folder := range payload.Folders {
		folderID := uuid.New().String()
		_, err = tx.Exec(`
			INSERT INTO libraryFolders (id, path, libraryId, createdAt, updatedAt)
			VALUES (?, ?, ?, ?, ?)`,
			folderID, folder.Path, libraryID, nowStr, nowStr)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if watcher.GlobalWatcher != nil {
		watcher.GlobalWatcher.Reload()
	}

	return GetLibraryByID(db, libraryID)
}

func UpdateLibrary(db *sql.DB, libraryID string, payload *UpdateLibraryPayload) (*LibraryJSON, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var currentMediaType, currentSettingsStr string
	err = tx.QueryRow("SELECT mediaType, settings FROM libraries WHERE id = ?", libraryID).Scan(&currentMediaType, &currentSettingsStr)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("library not found")
	}
	if err != nil {
		return nil, err
	}

	nowStr := time.Now().UTC().Format("2006-01-02 15:04:05.000")

	if payload.Name != nil {
		_, err = tx.Exec("UPDATE libraries SET name = ?, updatedAt = ? WHERE id = ?", *payload.Name, nowStr, libraryID)
		if err != nil {
			return nil, err
		}
	}
	if payload.Provider != nil {
		_, err = tx.Exec("UPDATE libraries SET provider = ?, updatedAt = ? WHERE id = ?", *payload.Provider, nowStr, libraryID)
		if err != nil {
			return nil, err
		}
	}
	if payload.MediaType != nil {
		_, err = tx.Exec("UPDATE libraries SET mediaType = ?, updatedAt = ? WHERE id = ?", *payload.MediaType, nowStr, libraryID)
		if err != nil {
			return nil, err
		}
		currentMediaType = *payload.MediaType
	}
	if payload.Icon != nil {
		_, err = tx.Exec("UPDATE libraries SET icon = ?, updatedAt = ? WHERE id = ?", *payload.Icon, nowStr, libraryID)
		if err != nil {
			return nil, err
		}
	}
	if payload.DisplayOrder != nil {
		_, err = tx.Exec("UPDATE libraries SET displayOrder = ?, updatedAt = ? WHERE id = ?", *payload.DisplayOrder, nowStr, libraryID)
		if err != nil {
			return nil, err
		}
	}

	if payload.Settings != nil {
		var currentSettings map[string]interface{}
		if err := json.Unmarshal([]byte(currentSettingsStr), &currentSettings); err != nil {
			currentSettings = getDefaultSettings(currentMediaType)
		}
		merged, err := mergeSettings(currentMediaType, payload.Settings)
		if err != nil {
			return nil, err
		}
		for k, v := range merged {
			currentSettings[k] = v
		}
		settingsBytes, err := json.Marshal(currentSettings)
		if err != nil {
			return nil, err
		}
		_, err = tx.Exec("UPDATE libraries SET settings = ?, updatedAt = ? WHERE id = ?", settingsBytes, nowStr, libraryID)
		if err != nil {
			return nil, err
		}
	}

	if payload.Folders != nil {
		rows, err := tx.Query("SELECT id, path FROM libraryFolders WHERE libraryId = ?", libraryID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		existingFolders := make(map[string]string)
		for rows.Next() {
			var fid, fpath string
			if err := rows.Scan(&fid, &fpath); err == nil {
				existingFolders[fid] = fpath
			}
		}
		rows.Close()

		inputFolderIDs := make(map[string]bool)
		for _, folder := range payload.Folders {
			if folder.ID != "" && existingFolders[folder.ID] != "" {
				inputFolderIDs[folder.ID] = true
			} else {
				folderID := uuid.New().String()
				_, err = tx.Exec(`
					INSERT INTO libraryFolders (id, path, libraryId, createdAt, updatedAt)
					VALUES (?, ?, ?, ?, ?)`,
					folderID, folder.Path, libraryID, nowStr, nowStr)
				if err != nil {
					return nil, err
				}
			}
		}

		hasMediaProgresses := tableExistsTx(tx, "mediaProgresses")
		hasPlaylistItems := tableExistsTx(tx, "playlistItems")
		for fid, fpath := range existingFolders {
			if !inputFolderIDs[fid] {
				_, err = tx.Exec("DELETE FROM libraryFolders WHERE id = ?", fid)
				if err != nil {
					return nil, err
				}

				itemRows, err := tx.Query("SELECT id, mediaId, mediaType FROM libraryItems WHERE libraryFolderId = ?", fid)
				if err == nil {
					type itemInfo struct {
						id        string
						mediaID   string
						mediaType string
					}
					var itemsToClean []itemInfo
					for itemRows.Next() {
						var item itemInfo
						if err := itemRows.Scan(&item.id, &item.mediaID, &item.mediaType); err == nil {
							itemsToClean = append(itemsToClean, item)
						}
					}
					itemRows.Close()

					for _, item := range itemsToClean {
						if hasMediaProgresses {
							_, _ = tx.Exec("DELETE FROM mediaProgresses WHERE mediaItemId = ?", item.mediaID)
						}
						if hasPlaylistItems {
							_, _ = tx.Exec("DELETE FROM playlistItems WHERE libraryItemId = ?", item.id)
						}
						_, _ = tx.Exec("DELETE FROM libraryItems WHERE id = ?", item.id)
					}
				}
				_ = fpath
			}
		}

		_, _ = tx.Exec("DELETE FROM books WHERE id NOT IN (SELECT mediaId FROM libraryItems WHERE mediaType = 'book')")
		_, _ = tx.Exec("DELETE FROM podcasts WHERE id NOT IN (SELECT mediaId FROM libraryItems WHERE mediaType = 'podcast')")
		if tableExistsTx(tx, "bookAuthors") {
			_, _ = tx.Exec("DELETE FROM bookAuthors WHERE bookId NOT IN (SELECT id FROM books)")
		}
		if tableExistsTx(tx, "bookSeries") {
			_, _ = tx.Exec("DELETE FROM bookSeries WHERE bookId NOT IN (SELECT id FROM books)")
		}
		if tableExistsTx(tx, "bookAuthors") && tableExistsTx(tx, "authors") {
			_, _ = tx.Exec("DELETE FROM authors WHERE id NOT IN (SELECT authorId FROM bookAuthors) AND (asin IS NULL OR asin = '') AND (description IS NULL OR description = '') AND (imagePath IS NULL OR imagePath = '')")
		}
		if tableExistsTx(tx, "bookSeries") && tableExistsTx(tx, "series") {
			_, _ = tx.Exec("DELETE FROM series WHERE id NOT IN (SELECT seriesId FROM bookSeries)")
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if watcher.GlobalWatcher != nil {
		watcher.GlobalWatcher.Reload()
	}

	return GetLibraryByID(db, libraryID)
}

func DeleteLibrary(db *sql.DB, libraryID string) (*LibraryJSON, error) {
	lib, err := GetLibraryByID(db, libraryID)
	if err != nil {
		return nil, err
	}
	if lib == nil {
		return nil, fmt.Errorf("library not found")
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if tableExistsTx(tx, "collections") {
		_, err = tx.Exec("DELETE FROM collections WHERE libraryId = ?", libraryID)
		if err != nil {
			return nil, err
		}
	}

	if tableExistsTx(tx, "playbackSessions") {
		_, err = tx.Exec("UPDATE playbackSessions SET libraryId = NULL WHERE libraryId = ?", libraryID)
		if err != nil {
			return nil, err
		}
	}

	itemRows, err := tx.Query("SELECT id, mediaId, mediaType FROM libraryItems WHERE libraryId = ?", libraryID)
	if err == nil {
		type itemInfo struct {
			id        string
			mediaID   string
			mediaType string
		}
		var itemsToClean []itemInfo
		for itemRows.Next() {
			var item itemInfo
			if err := itemRows.Scan(&item.id, &item.mediaID, &item.mediaType); err == nil {
				itemsToClean = append(itemsToClean, item)
			}
		}
		itemRows.Close()

		hasMediaProgresses := tableExistsTx(tx, "mediaProgresses")
		hasPlaylistItems := tableExistsTx(tx, "playlistItems")
		for _, item := range itemsToClean {
			if hasMediaProgresses {
				_, _ = tx.Exec("DELETE FROM mediaProgresses WHERE mediaItemId = ?", item.mediaID)
			}
			if hasPlaylistItems {
				_, _ = tx.Exec("DELETE FROM playlistItems WHERE libraryItemId = ?", item.id)
			}
			_, _ = tx.Exec("DELETE FROM libraryItems WHERE id = ?", item.id)
		}
	}

	_, err = tx.Exec("DELETE FROM libraryFolders WHERE libraryId = ?", libraryID)
	if err != nil {
		return nil, err
	}

	_, _ = tx.Exec("DELETE FROM books WHERE id NOT IN (SELECT mediaId FROM libraryItems WHERE mediaType = 'book')")
	_, _ = tx.Exec("DELETE FROM podcasts WHERE id NOT IN (SELECT mediaId FROM libraryItems WHERE mediaType = 'podcast')")
	if tableExistsTx(tx, "bookAuthors") {
		_, _ = tx.Exec("DELETE FROM bookAuthors WHERE bookId NOT IN (SELECT id FROM books)")
	}
	if tableExistsTx(tx, "bookSeries") {
		_, _ = tx.Exec("DELETE FROM bookSeries WHERE bookId NOT IN (SELECT id FROM books)")
	}
	if tableExistsTx(tx, "bookAuthors") && tableExistsTx(tx, "authors") {
		_, _ = tx.Exec("DELETE FROM authors WHERE id NOT IN (SELECT authorId FROM bookAuthors) AND (asin IS NULL OR asin = '') AND (description IS NULL OR description = '') AND (imagePath IS NULL OR imagePath = '')")
	}
	if tableExistsTx(tx, "bookSeries") && tableExistsTx(tx, "series") {
		_, _ = tx.Exec("DELETE FROM series WHERE id NOT IN (SELECT seriesId FROM bookSeries)")
	}

	_, err = tx.Exec("DELETE FROM libraries WHERE id = ?", libraryID)
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query("SELECT id FROM libraries ORDER BY displayOrder ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var remainingIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			remainingIDs = append(remainingIDs, id)
		}
	}
	rows.Close()

	for i, id := range remainingIDs {
		_, err = tx.Exec("UPDATE libraries SET displayOrder = ? WHERE id = ?", i+1, id)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if watcher.GlobalWatcher != nil {
		watcher.GlobalWatcher.Reload()
	}

	return lib, nil
}

type LibraryFilterData struct {
	Authors          []map[string]string `json:"authors"`
	Genres           []string            `json:"genres"`
	Tags             []string            `json:"tags"`
	Series           []map[string]string `json:"series"`
	Narrators        []string            `json:"narrators"`
	Languages        []string            `json:"languages"`
	Publishers       []string            `json:"publishers"`
	PublishedDecades []string            `json:"publishedDecades"`
	BookCount        int                 `json:"bookCount"`
	AuthorCount      int                 `json:"authorCount"`
	SeriesCount      int                 `json:"seriesCount"`
	PodcastCount     int                 `json:"podcastCount"`
	NumIssues        int                 `json:"numIssues"`
}

func GetLibraryFilterDataGo(db *sql.DB, libraryID string) (*LibraryFilterData, error) {
	var mediaType string
	err := db.QueryRow("SELECT mediaType FROM libraries WHERE id = ?", libraryID).Scan(&mediaType)
	if err != nil {
		return nil, err
	}

	fd := &LibraryFilterData{
		Authors:          []map[string]string{},
		Genres:           []string{},
		Tags:             []string{},
		Series:           []map[string]string{},
		Narrators:        []string{},
		Languages:        []string{},
		Publishers:       []string{},
		PublishedDecades: []string{},
	}

	genresSet := make(map[string]bool)
	tagsSet := make(map[string]bool)
	narratorsSet := make(map[string]bool)
	languagesSet := make(map[string]bool)
	publishersSet := make(map[string]bool)
	decadesSet := make(map[string]bool)

	if mediaType == "podcast" {
		rows, err := db.Query(`SELECT p.tags, p.genres, p.language 
			FROM podcasts p JOIN libraryItems li ON li.mediaId = p.id WHERE li.libraryId = ?`, libraryID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var tagsStr, genresStr, langStr sql.NullString
				if err := rows.Scan(&tagsStr, &genresStr, &langStr); err == nil {
					if tagsStr.Valid && tagsStr.String != "" {
						var arr []string
						if json.Unmarshal([]byte(tagsStr.String), &arr) == nil {
							for _, v := range arr {
								if v != "" {
									tagsSet[v] = true
								}
							}
						}
					}
					if genresStr.Valid && genresStr.String != "" {
						var arr []string
						if json.Unmarshal([]byte(genresStr.String), &arr) == nil {
							for _, v := range arr {
								if v != "" {
									genresSet[v] = true
								}
							}
						}
					}
					if langStr.Valid && langStr.String != "" {
						languagesSet[langStr.String] = true
					}
				}
			}
		}

		db.QueryRow("SELECT COUNT(*) FROM libraryItems WHERE libraryId = ?", libraryID).Scan(&fd.PodcastCount)

	} else {
		rows, err := db.Query(`SELECT b.tags, b.genres, b.narrators, b.publisher, b.publishedYear, b.language, li.isMissing, li.isInvalid 
			FROM books b JOIN libraryItems li ON li.mediaId = b.id WHERE li.libraryId = ?`, libraryID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var tagsStr, genresStr, narrStr, pubStr, langStr sql.NullString
				var pubYear sql.NullInt64
				var isMissingVal, isInvalidVal int
				if err := rows.Scan(&tagsStr, &genresStr, &narrStr, &pubStr, &pubYear, &langStr, &isMissingVal, &isInvalidVal); err == nil {
					if isMissingVal != 0 || isInvalidVal != 0 {
						fd.NumIssues++
					}
					if tagsStr.Valid && tagsStr.String != "" {
						var arr []string
						if json.Unmarshal([]byte(tagsStr.String), &arr) == nil {
							for _, v := range arr {
								if v != "" {
									tagsSet[v] = true
								}
							}
						}
					}
					if genresStr.Valid && genresStr.String != "" {
						var arr []string
						if json.Unmarshal([]byte(genresStr.String), &arr) == nil {
							for _, v := range arr {
								if v != "" {
									genresSet[v] = true
								}
							}
						}
					}
					if narrStr.Valid && narrStr.String != "" {
						var arr []string
						if json.Unmarshal([]byte(narrStr.String), &arr) == nil {
							for _, v := range arr {
								if v != "" {
									narratorsSet[v] = true
								}
							}
						}
					}
					if pubStr.Valid && pubStr.String != "" {
						publishersSet[pubStr.String] = true
					}
					if langStr.Valid && langStr.String != "" {
						languagesSet[langStr.String] = true
					}
					if pubYear.Valid && pubYear.Int64 > 0 && pubYear.Int64 < 3000 {
						decade := (pubYear.Int64 / 10) * 10
						decadesSet[strconv.FormatInt(decade, 10)] = true
					}
				}
			}
		}

		db.QueryRow("SELECT COUNT(*) FROM libraryItems WHERE libraryId = ?", libraryID).Scan(&fd.BookCount)
		db.QueryRow("SELECT COUNT(*) FROM series WHERE libraryId = ?", libraryID).Scan(&fd.SeriesCount)
		db.QueryRow("SELECT COUNT(*) FROM authors WHERE libraryId = ?", libraryID).Scan(&fd.AuthorCount)

		// Get authors list
		authRows, err := db.Query("SELECT id, name FROM authors WHERE libraryId = ?", libraryID)
		if err == nil {
			defer authRows.Close()
			for authRows.Next() {
				var id, name string
				if err := authRows.Scan(&id, &name); err == nil {
					fd.Authors = append(fd.Authors, map[string]string{"id": id, "name": name})
				}
			}
			sort.Slice(fd.Authors, func(i, j int) bool {
				return strings.ToLower(fd.Authors[i]["name"]) < strings.ToLower(fd.Authors[j]["name"])
			})
		}

		// Get series list
		serRows, err := db.Query("SELECT id, name FROM series WHERE libraryId = ?", libraryID)
		if err == nil {
			defer serRows.Close()
			for serRows.Next() {
				var id, name string
				if err := serRows.Scan(&id, &name); err == nil {
					fd.Series = append(fd.Series, map[string]string{"id": id, "name": name})
				}
			}
			sort.Slice(fd.Series, func(i, j int) bool {
				return strings.ToLower(fd.Series[i]["name"]) < strings.ToLower(fd.Series[j]["name"])
			})
		}
	}

	for k := range genresSet {
		fd.Genres = append(fd.Genres, k)
	}
	sort.Strings(fd.Genres)

	for k := range tagsSet {
		fd.Tags = append(fd.Tags, k)
	}
	sort.Strings(fd.Tags)

	for k := range narratorsSet {
		fd.Narrators = append(fd.Narrators, k)
	}
	sort.Strings(fd.Narrators)

	for k := range languagesSet {
		fd.Languages = append(fd.Languages, k)
	}
	sort.Strings(fd.Languages)

	for k := range publishersSet {
		fd.Publishers = append(fd.Publishers, k)
	}
	sort.Strings(fd.Publishers)

	for k := range decadesSet {
		fd.PublishedDecades = append(fd.PublishedDecades, k)
	}
	sort.Strings(fd.PublishedDecades)

	return fd, nil
}

// nullIfEmpty returns nil if s is empty, otherwise returns a pointer to s.
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
