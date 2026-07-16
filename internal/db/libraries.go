package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	watcher "audiobookshelf/internal/watcher"
)

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

type GenreWithCount struct {
	Genre string `json:"genre"`
	Count int    `json:"count"`
}

type AuthorWithCount struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type MinLibraryItem struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Duration float64 `json:"duration,omitempty"`
	Size     int64   `json:"size,omitempty"`
}

type LibraryStats struct {
	TotalSize        int64             `json:"totalSize"`
	TotalDuration    float64           `json:"totalDuration"`
	NumAudioFiles    int               `json:"numAudioFiles"`
	NumAudioTracks   int               `json:"numAudioTracks"`
	TotalItems       int               `json:"totalItems"`
	TotalAuthors     int               `json:"totalAuthors"`
	GenresWithCount  []GenreWithCount  `json:"genresWithCount"`
	AuthorsWithCount []AuthorWithCount `json:"authorsWithCount"`
	LongestItems     []MinLibraryItem  `json:"longestItems"`
	LargestItems     []MinLibraryItem  `json:"largestItems"`
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
	stats.NumAudioTracks = stats.NumAudioFiles

	// Get total authors
	err = db.QueryRow(`
		SELECT COUNT(DISTINCT ba.authorId)
		FROM libraryItems li
		JOIN bookAuthors ba ON ba.bookId = li.mediaId AND li.mediaType = 'book'
		WHERE li.libraryId = ?
	`, libraryID).Scan(&stats.TotalAuthors)
	if err != nil {
		stats.TotalAuthors = 0
	}

	// Genres
	rows, err := db.Query(`
		SELECT json_each.value AS genre, COUNT(*) AS count
		FROM libraryItems li
		JOIN books b ON b.id = li.mediaId AND li.mediaType = 'book'
		JOIN json_each(b.genres)
		WHERE li.libraryId = ? AND json_valid(b.genres)
		GROUP BY genre
		ORDER BY count DESC, genre ASC
	`, libraryID)
	stats.GenresWithCount = []GenreWithCount{}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var g GenreWithCount
			if err := rows.Scan(&g.Genre, &g.Count); err == nil {
				stats.GenresWithCount = append(stats.GenresWithCount, g)
			}
		}
	}

	// Authors
	rows, err = db.Query(`
		SELECT a.id, a.name, COUNT(*) AS count
		FROM libraryItems li
		JOIN bookAuthors ba ON ba.bookId = li.mediaId AND li.mediaType = 'book'
		JOIN authors a ON a.id = ba.authorId
		WHERE li.libraryId = ?
		GROUP BY a.id, a.name
		ORDER BY count DESC, a.name ASC
	`, libraryID)
	stats.AuthorsWithCount = []AuthorWithCount{}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var a AuthorWithCount
			if err := rows.Scan(&a.ID, &a.Name, &a.Count); err == nil {
				stats.AuthorsWithCount = append(stats.AuthorsWithCount, a)
			}
		}
	}

	// Longest Items
	rows, err = db.Query(`
		SELECT li.mediaId, li.title, b.duration
		FROM libraryItems li
		JOIN books b ON b.id = li.mediaId AND li.mediaType = 'book'
		WHERE li.libraryId = ?
		ORDER BY b.duration DESC
		LIMIT 10
	`, libraryID)
	stats.LongestItems = []MinLibraryItem{}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var item MinLibraryItem
			if err := rows.Scan(&item.ID, &item.Title, &item.Duration); err == nil {
				stats.LongestItems = append(stats.LongestItems, item)
			}
		}
	}

	// Largest Items
	rows, err = db.Query(`
		SELECT li.mediaId, li.title, li.size
		FROM libraryItems li
		WHERE li.libraryId = ? AND li.mediaType = 'book'
		ORDER BY li.size DESC
		LIMIT 10
	`, libraryID)
	stats.LargestItems = []MinLibraryItem{}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var item MinLibraryItem
			if err := rows.Scan(&item.ID, &item.Title, &item.Size); err == nil {
				stats.LargestItems = append(stats.LargestItems, item)
			}
		}
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
	stats.TotalAuthors = 0
	stats.AuthorsWithCount = []AuthorWithCount{}

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
	stats.NumAudioTracks = stats.NumAudioFiles

	// Genres
	rows, err := db.Query(`
		SELECT json_each.value AS genre, COUNT(*) AS count
		FROM libraryItems li
		JOIN podcasts p ON p.id = li.mediaId AND li.mediaType = 'podcast'
		JOIN json_each(p.genres)
		WHERE li.libraryId = ? AND json_valid(p.genres)
		GROUP BY genre
		ORDER BY count DESC, genre ASC
	`, libraryID)
	stats.GenresWithCount = []GenreWithCount{}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var g GenreWithCount
			if err := rows.Scan(&g.Genre, &g.Count); err == nil {
				stats.GenresWithCount = append(stats.GenresWithCount, g)
			}
		}
	}

	// Longest Items
	rows, err = db.Query(`
		SELECT li.mediaId AS id, li.title, COALESCE(SUM(CAST(json_extract(pe.audioFile, '$.duration') AS REAL)), 0) AS duration
		FROM libraryItems li
		JOIN podcasts p ON p.id = li.mediaId AND li.mediaType = 'podcast'
		LEFT JOIN podcastEpisodes pe ON pe.podcastId = p.id
		WHERE li.libraryId = ?
		GROUP BY li.id
		ORDER BY duration DESC
		LIMIT 10
	`, libraryID)
	stats.LongestItems = []MinLibraryItem{}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var item MinLibraryItem
			if err := rows.Scan(&item.ID, &item.Title, &item.Duration); err == nil {
				stats.LongestItems = append(stats.LongestItems, item)
			}
		}
	}

	// Largest Items
	rows, err = db.Query(`
		SELECT li.mediaId AS id, li.title, li.size
		FROM libraryItems li
		WHERE li.libraryId = ? AND li.mediaType = 'podcast'
		ORDER BY li.size DESC
		LIMIT 10
	`, libraryID)
	stats.LargestItems = []MinLibraryItem{}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var item MinLibraryItem
			if err := rows.Scan(&item.ID, &item.Title, &item.Size); err == nil {
				stats.LargestItems = append(stats.LargestItems, item)
			}
		}
	}

	return &stats, nil
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
				if existingFolders[folder.ID] != folder.Path {
					_, err = tx.Exec(`
						UPDATE libraryFolders SET path = ?, updatedAt = ?
						WHERE id = ?`,
						folder.Path, nowStr, folder.ID)
					if err != nil {
						return nil, err
					}
				}
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

				if hasMediaProgresses {
					_, _ = tx.Exec("DELETE FROM mediaProgresses WHERE mediaItemId IN (SELECT mediaId FROM libraryItems WHERE libraryFolderId = ?)", fid)
				}
				if hasPlaylistItems {
					_, _ = tx.Exec("DELETE FROM playlistItems WHERE libraryItemId IN (SELECT id FROM libraryItems WHERE libraryFolderId = ?)", fid)
				}
				_, err = tx.Exec("DELETE FROM libraryItems WHERE libraryFolderId = ?", fid)
				if err != nil {
					return nil, err
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

	hasMediaProgresses := tableExistsTx(tx, "mediaProgresses")
	hasPlaylistItems := tableExistsTx(tx, "playlistItems")
	if hasMediaProgresses {
		_, _ = tx.Exec("DELETE FROM mediaProgresses WHERE mediaItemId IN (SELECT mediaId FROM libraryItems WHERE libraryId = ?)", libraryID)
	}
	if hasPlaylistItems {
		_, _ = tx.Exec("DELETE FROM playlistItems WHERE libraryItemId IN (SELECT id FROM libraryItems WHERE libraryId = ?)", libraryID)
	}
	_, err = tx.Exec("DELETE FROM libraryItems WHERE libraryId = ?", libraryID)
	if err != nil {
		return nil, err
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
