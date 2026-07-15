package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/utils"
)

// Shelf represents a shelf row on the personalized page.
type Shelf struct {
	ID             string                         `json:"id"`
	Label          string                         `json:"label"`
	LabelStringKey string                         `json:"labelStringKey"`
	Type           string                         `json:"type"`
	Entities       []*idb.LibraryItemMinifiedJSON `json:"entities"`
}

func HandleGetLibraryItems(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		lib, err := idb.GetLibraryByID(db, libraryID)
		if err != nil {
			log.Errorf("[Go] Failed to get library %s: %v", libraryID, err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		if lib == nil {
			http.Error(w, "Library not found", http.StatusNotFound)
			return
		}

		q := r.URL.Query()
		limitVal := 0
		if q.Get("limit") != "" {
			fmt.Sscanf(q.Get("limit"), "%d", &limitVal)
		}
		pageVal := 0
		if q.Get("page") != "" {
			fmt.Sscanf(q.Get("page"), "%d", &pageVal)
		}

		sortBy := q.Get("sort")
		sortDesc := q.Get("desc") == "1"
		filterBy := q.Get("filter")
		minified := q.Get("minified") == "1"
		collapseseries := q.Get("collapseseries") == "1"
		include := q.Get("include")
		searchQuery := q.Get("search")

		var includeArray []string
		if include != "" {
			for _, part := range strings.Split(include, ",") {
				includeArray = append(includeArray, strings.TrimSpace(part))
			}
		}

		opts := idb.GetFilteredLibraryItemsOptions{
			LibraryID:      libraryID,
			User:           user,
			FilterBy:       filterBy,
			SortBy:         sortBy,
			SortDesc:       sortDesc,
			Limit:          limitVal,
			Page:           pageVal,
			CollapseSeries: collapseseries,
			Include:        includeArray,
			MediaType:      lib.MediaType,
			Minified:       minified,
			Search:         searchQuery,
		}

		results, total, err := idb.GetFilteredLibraryItems(db, opts)
		if err != nil {
			log.Errorf("[Go] Failed to get filtered items for library %s: %v", libraryID, err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		payload := map[string]interface{}{
			"results":        results,
			"total":          total,
			"limit":          limitVal,
			"page":           pageVal,
			"sortBy":         sortBy,
			"sortDesc":       sortDesc,
			"filterBy":       filterBy,
			"mediaType":      lib.MediaType,
			"minified":       minified,
			"collapseseries": collapseseries,
			"include":        include,
			"search":         searchQuery,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

func fetchProgressShelves(db *sql.DB, libraryID string, user *core.UserSession, limitVal int, mediaType string) ([]Shelf, error) {
	var shelves []Shelf
	optsProgress := idb.GetFilteredLibraryItemsOptions{
		LibraryID: libraryID,
		User:      user,
		FilterBy:  "progress.in-progress",
		SortBy:    "progress",
		SortDesc:  true,
		Limit:     limitVal,
		Page:      0,
		MediaType: mediaType,
		Minified:  true,
	}
	progressItems, _, err := idb.GetFilteredLibraryItems(db, optsProgress)
	if err != nil {
		return nil, err
	}

	if len(progressItems) > 0 {
		if mediaType == "book" {
			var listeningItems []*idb.LibraryItemMinifiedJSON
			var readingItems []*idb.LibraryItemMinifiedJSON

			for _, item := range progressItems {
				if item.IsMissing || item.IsInvalid {
					continue
				}
				bookMin, ok := item.Media.(*idb.BookMinifiedJSON)
				if ok && bookMin.NumAudioFiles > 0 {
					listeningItems = append(listeningItems, item)
				} else {
					readingItems = append(readingItems, item)
				}
			}

			if len(listeningItems) > 0 {
				shelves = append(shelves, Shelf{
					ID:             "continue-listening",
					Label:          "Continue Listening",
					LabelStringKey: "LabelContinueListening",
					Type:           "book",
					Entities:       listeningItems,
				})
			}
			if len(readingItems) > 0 {
				shelves = append(shelves, Shelf{
					ID:             "continue-reading",
					Label:          "Continue Reading",
					LabelStringKey: "LabelContinueReading",
					Type:           "book",
					Entities:       readingItems,
				})
			}
		} else if mediaType == "podcast" {
			var filteredProgress []*idb.LibraryItemMinifiedJSON
			for _, item := range progressItems {
				if item.IsMissing || item.IsInvalid {
					continue
				}
				filteredProgress = append(filteredProgress, item)
			}
			if len(filteredProgress) > 0 {
				shelves = append(shelves, Shelf{
					ID:             "continue-listening",
					Label:          "Continue Listening",
					LabelStringKey: "LabelContinueListening",
					Type:           "episode",
					Entities:       filteredProgress,
				})
			}
		}
	}
	return shelves, nil
}

func fetchRecentlyAddedShelf(db *sql.DB, libraryID string, user *core.UserSession, limitVal int, mediaType string) (*Shelf, error) {
	optsRecent := idb.GetFilteredLibraryItemsOptions{
		LibraryID: libraryID,
		User:      user,
		SortBy:    "addedAt",
		SortDesc:  true,
		Limit:     limitVal,
		Page:      0,
		MediaType: mediaType,
		Minified:  true,
	}
	recentItems, _, err := idb.GetFilteredLibraryItems(db, optsRecent)
	if err != nil {
		return nil, err
	}

	if len(recentItems) > 0 {
		var filteredRecent []*idb.LibraryItemMinifiedJSON
		for _, item := range recentItems {
			if item.IsMissing || item.IsInvalid {
				continue
			}
			filteredRecent = append(filteredRecent, item)
		}
		if len(filteredRecent) > 0 {
			return &Shelf{
				ID:             "recently-added",
				Label:          "Recently Added",
				LabelStringKey: "LabelRecentlyAdded",
				Type:           mediaType,
				Entities:       filteredRecent,
			}, nil
		}
	}
	return nil, nil
}

func fetchContinueSeriesShelf(db *sql.DB, libraryID string, user *core.UserSession, limitVal int) (*Shelf, error) {
	query := `
		WITH FinishedBooks AS (
			SELECT bs.seriesId, bs.sequence AS finished_seq
			FROM mediaProgresses mp
			JOIN libraryItems li ON mp.mediaItemId = li.id
			JOIN bookSeries bs ON li.mediaId = bs.bookId
			WHERE mp.userId = ? AND mp.isFinished = 1 AND li.libraryId = ? AND li.mediaType = 'book'
		),
		NextBooks AS (
			SELECT li.id AS library_item_id, bs.seriesId, bs.sequence,
			       ROW_NUMBER() OVER (PARTITION BY bs.seriesId ORDER BY CAST(bs.sequence AS REAL) ASC) as rn
			FROM bookSeries bs
			JOIN libraryItems li ON bs.bookId = li.mediaId
			JOIN FinishedBooks fb ON bs.seriesId = fb.seriesId
			LEFT JOIN mediaProgresses mp ON li.id = mp.mediaItemId AND mp.userId = ?
			WHERE li.libraryId = ? AND li.mediaType = 'book'
			  AND CAST(bs.sequence AS REAL) > CAST(fb.finished_seq AS REAL)
			  AND (mp.id IS NULL OR mp.isFinished = 0)
		)
		SELECT library_item_id FROM NextBooks WHERE rn = 1
	`
	rows, err := db.Query(query, user.ID, libraryID, user.ID, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var itemIDs []string
	for rows.Next() {
		var itemID string
		if err := rows.Scan(&itemID); err == nil {
			itemIDs = append(itemIDs, itemID)
		}
	}

	if len(itemIDs) == 0 {
		return nil, nil
	}

	opts := idb.GetFilteredLibraryItemsOptions{
		LibraryID: libraryID,
		User:      user,
		Include:   itemIDs,
		MediaType: "book",
		Minified:  true,
		Limit:     limitVal,
	}
	items, _, err := idb.GetFilteredLibraryItems(db, opts)
	if err != nil {
		return nil, err
	}

	if len(items) == 0 {
		return nil, nil
	}

	return &Shelf{
		ID:             "continue-series",
		Label:          "Continue Series",
		LabelStringKey: "LabelContinueSeries",
		Type:           "book",
		Entities:       items,
	}, nil
}

func fetchRecentSeriesShelf(db *sql.DB, libraryID string, user *core.UserSession, limitVal int) (*Shelf, error) {
	query := `
		SELECT li.id
		FROM libraryItems li
		JOIN bookSeries bs ON li.mediaId = bs.bookId
		WHERE li.libraryId = ? AND li.mediaType = 'book'
		GROUP BY bs.seriesId
		ORDER BY MAX(li.addedAt) DESC
		LIMIT ?
	`
	rows, err := db.Query(query, libraryID, limitVal)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var itemIDs []string
	for rows.Next() {
		var itemID string
		if err := rows.Scan(&itemID); err == nil {
			itemIDs = append(itemIDs, itemID)
		}
	}

	if len(itemIDs) == 0 {
		return nil, nil
	}

	opts := idb.GetFilteredLibraryItemsOptions{
		LibraryID: libraryID,
		User:      user,
		Include:   itemIDs,
		MediaType: "book",
		Minified:  true,
		Limit:     limitVal,
	}
	items, _, err := idb.GetFilteredLibraryItems(db, opts)
	if err != nil {
		return nil, err
	}

	itemMap := make(map[string]*idb.LibraryItemMinifiedJSON)
	for _, item := range items {
		itemMap[item.ID] = item
	}

	var filteredItems []*idb.LibraryItemMinifiedJSON
	for _, itemID := range itemIDs {
		item, exists := itemMap[itemID]
		if !exists || item.IsMissing || item.IsInvalid {
			continue
		}
		filteredItems = append(filteredItems, item)
	}

	if len(filteredItems) == 0 {
		return nil, nil
	}

	return &Shelf{
		ID:             "recent-series",
		Label:          "Recent Series",
		LabelStringKey: "LabelRecentSeries",
		Type:           "book",
		Entities:       filteredItems,
	}, nil
}

func fetchDiscoverShelf(db *sql.DB, libraryID string, user *core.UserSession, limitVal int, mediaType string) (*Shelf, error) {
	query := `
		SELECT li.id
		FROM libraryItems li
		LEFT JOIN mediaProgresses mp ON li.id = mp.mediaItemId AND mp.userId = ?
		WHERE li.libraryId = ? AND li.mediaType = ? AND (mp.isFinished IS NULL OR mp.isFinished = 0)
		ORDER BY RANDOM()
		LIMIT ?
	`
	rows, err := db.Query(query, user.ID, libraryID, mediaType, limitVal)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var itemIDs []string
	for rows.Next() {
		var itemID string
		if err := rows.Scan(&itemID); err == nil {
			itemIDs = append(itemIDs, itemID)
		}
	}

	if len(itemIDs) == 0 {
		fallbackQuery := `
			SELECT id FROM libraryItems
			WHERE libraryId = ? AND mediaType = ?
			ORDER BY RANDOM()
			LIMIT ?
		`
		fallbackRows, err := db.Query(fallbackQuery, libraryID, mediaType, limitVal)
		if err != nil {
			return nil, err
		}
		defer fallbackRows.Close()
		for fallbackRows.Next() {
			var itemID string
			if err := fallbackRows.Scan(&itemID); err == nil {
				itemIDs = append(itemIDs, itemID)
			}
		}
	}

	if len(itemIDs) == 0 {
		return nil, nil
	}

	opts := idb.GetFilteredLibraryItemsOptions{
		LibraryID: libraryID,
		User:      user,
		Include:   itemIDs,
		MediaType: mediaType,
		Minified:  true,
		Limit:     limitVal,
	}
	items, _, err := idb.GetFilteredLibraryItems(db, opts)
	if err != nil {
		return nil, err
	}

	var filteredItems []*idb.LibraryItemMinifiedJSON
	for _, item := range items {
		if item.IsMissing || item.IsInvalid {
			continue
		}
		filteredItems = append(filteredItems, item)
	}

	if len(filteredItems) == 0 {
		return nil, nil
	}

	return &Shelf{
		ID:             "discover",
		Label:          "Discover",
		LabelStringKey: "LabelDiscover",
		Type:           mediaType,
		Entities:       filteredItems,
	}, nil
}

func fetchFinishedShelves(db *sql.DB, libraryID string, user *core.UserSession, limitVal int, mediaType string) ([]Shelf, error) {
	var shelves []Shelf
	if mediaType != "book" {
		return nil, nil
	}

	query := `
		SELECT li.id
		FROM mediaProgresses mp
		JOIN libraryItems li ON mp.mediaItemId = li.id
		WHERE mp.userId = ? AND mp.isFinished = 1 AND li.libraryId = ? AND li.mediaType = 'book'
		ORDER BY mp.updatedAt DESC
		LIMIT ?
	`
	rows, err := db.Query(query, user.ID, libraryID, limitVal)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var itemIDs []string
	for rows.Next() {
		var itemID string
		if err := rows.Scan(&itemID); err == nil {
			itemIDs = append(itemIDs, itemID)
		}
	}

	if len(itemIDs) == 0 {
		return nil, nil
	}

	opts := idb.GetFilteredLibraryItemsOptions{
		LibraryID: libraryID,
		User:      user,
		Include:   itemIDs,
		MediaType: "book",
		Minified:  true,
		Limit:     limitVal,
	}
	items, _, err := idb.GetFilteredLibraryItems(db, opts)
	if err != nil {
		return nil, err
	}

	itemMap := make(map[string]*idb.LibraryItemMinifiedJSON)
	for _, item := range items {
		itemMap[item.ID] = item
	}

	var listeningItems []*idb.LibraryItemMinifiedJSON
	var readingItems []*idb.LibraryItemMinifiedJSON

	for _, itemID := range itemIDs {
		item, exists := itemMap[itemID]
		if !exists || item.IsMissing || item.IsInvalid {
			continue
		}
		bookMin, ok := item.Media.(*idb.BookMinifiedJSON)
		if ok && bookMin.NumAudioFiles > 0 {
			listeningItems = append(listeningItems, item)
		} else {
			readingItems = append(readingItems, item)
		}
	}

	if len(listeningItems) > 0 {
		shelves = append(shelves, Shelf{
			ID:             "listen-again",
			Label:          "Listen Again",
			LabelStringKey: "LabelListenAgain",
			Type:           "book",
			Entities:       listeningItems,
		})
	}
	if len(readingItems) > 0 {
		shelves = append(shelves, Shelf{
			ID:             "read-again",
			Label:          "Read Again",
			LabelStringKey: "LabelReadAgain",
			Type:           "book",
			Entities:       readingItems,
		})
	}
	return shelves, nil
}

func fetchNewestAuthorsShelf(db *sql.DB, libraryID string, user *core.UserSession, limitVal int) (*Shelf, error) {
	query := `
		SELECT li.id
		FROM libraryItems li
		JOIN bookAuthors ba ON li.mediaId = ba.bookId
		JOIN authors a ON ba.authorId = a.id
		WHERE li.libraryId = ? AND li.mediaType = 'book'
		GROUP BY a.id
		ORDER BY a.createdAt DESC
		LIMIT ?
	`
	rows, err := db.Query(query, libraryID, limitVal)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var itemIDs []string
	for rows.Next() {
		var itemID string
		if err := rows.Scan(&itemID); err == nil {
			itemIDs = append(itemIDs, itemID)
		}
	}

	if len(itemIDs) == 0 {
		return nil, nil
	}

	opts := idb.GetFilteredLibraryItemsOptions{
		LibraryID: libraryID,
		User:      user,
		Include:   itemIDs,
		MediaType: "book",
		Minified:  true,
		Limit:     limitVal,
	}
	items, _, err := idb.GetFilteredLibraryItems(db, opts)
	if err != nil {
		return nil, err
	}

	itemMap := make(map[string]*idb.LibraryItemMinifiedJSON)
	for _, item := range items {
		itemMap[item.ID] = item
	}

	var filteredItems []*idb.LibraryItemMinifiedJSON
	for _, itemID := range itemIDs {
		item, exists := itemMap[itemID]
		if !exists || item.IsMissing || item.IsInvalid {
			continue
		}
		filteredItems = append(filteredItems, item)
	}

	if len(filteredItems) == 0 {
		return nil, nil
	}

	return &Shelf{
		ID:             "newest-authors",
		Label:          "Newest Authors",
		LabelStringKey: "LabelNewestAuthors",
		Type:           "book",
		Entities:       filteredItems,
	}, nil
}

func HandleGetLibraryPersonalized(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		lib, err := idb.GetLibraryByID(db, libraryID)
		if err != nil {
			log.Errorf("[Go] Failed to get library %s: %v", libraryID, err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		if lib == nil {
			http.Error(w, "Library not found", http.StatusNotFound)
			return
		}

		q := r.URL.Query()
		limitVal := 20
		if q.Get("limit") != "" {
			fmt.Sscanf(q.Get("limit"), "%d", &limitVal)
		}

		var shelves []Shelf

		// 1. Fetch in-progress items
		progressShelves, err := fetchProgressShelves(db, libraryID, user, limitVal, lib.MediaType)
		if err == nil && len(progressShelves) > 0 {
			shelves = append(shelves, progressShelves...)
		}

		// 1.5. Fetch continue series items (books only)
		if lib.MediaType == "book" {
			seriesShelf, err := fetchContinueSeriesShelf(db, libraryID, user, limitVal)
			if err == nil && seriesShelf != nil {
				shelves = append(shelves, *seriesShelf)
			}
		}

		// 2. Fetch recently added items
		recentShelf, err := fetchRecentlyAddedShelf(db, libraryID, user, limitVal, lib.MediaType)
		if err == nil && recentShelf != nil {
			shelves = append(shelves, *recentShelf)
		}

		// 3. Fetch recent series (books only)
		if lib.MediaType == "book" {
			rsShelf, err := fetchRecentSeriesShelf(db, libraryID, user, limitVal)
			if err == nil && rsShelf != nil {
				shelves = append(shelves, *rsShelf)
			}
		}

		// 4. Fetch discover shelf
		discShelf, err := fetchDiscoverShelf(db, libraryID, user, limitVal, lib.MediaType)
		if err == nil && discShelf != nil {
			shelves = append(shelves, *discShelf)
		}

		// 5. Fetch finished shelves (Listen Again, Read Again)
		finishedShelves, err := fetchFinishedShelves(db, libraryID, user, limitVal, lib.MediaType)
		if err == nil && len(finishedShelves) > 0 {
			shelves = append(shelves, finishedShelves...)
		}

		// 6. Fetch newest authors
		if lib.MediaType == "book" {
			naShelf, err := fetchNewestAuthorsShelf(db, libraryID, user, limitVal)
			if err == nil && naShelf != nil {
				shelves = append(shelves, *naShelf)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if shelves == nil {
			shelves = []Shelf{}
		}
		json.NewEncoder(w).Encode(shelves)
	}
}

// HandleSearchLibrary handles GET /api/libraries/{libraryID}/search
func HandleSearchLibrary(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/libraries/%s/search", libraryID)

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

		q := r.URL.Query().Get("q")
		if q == "" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"book":      []interface{}{},
				"podcast":   []interface{}{},
				"episodes":  []interface{}{},
				"authors":   []interface{}{},
				"series":    []interface{}{},
				"tags":      []interface{}{},
				"genres":    []interface{}{},
				"narrators": []interface{}{},
			})
			return
		}

		limitVal := r.URL.Query().Get("limit")
		limit := 3
		if limitVal != "" {
			if l, err := strconv.Atoi(limitVal); err == nil && l > 0 {
				limit = l
			}
		}

		// 1. Books (MediaType: "book")
		var bookResults []map[string]interface{} = []map[string]interface{}{}
		optsBooks := idb.GetFilteredLibraryItemsOptions{
			LibraryID: libraryID,
			User:      user,
			Search:    q,
			Limit:     limit,
			MediaType: "book",
		}
		bookItems, _, err := idb.GetFilteredLibraryItems(db, optsBooks)
		if err == nil {
			for _, item := range bookItems {
				bookResults = append(bookResults, map[string]interface{}{
					"libraryItem": item,
				})
			}
		}

		// 2. Podcasts (MediaType: "podcast")
		var podcastResults []map[string]interface{} = []map[string]interface{}{}
		optsPodcasts := idb.GetFilteredLibraryItemsOptions{
			LibraryID: libraryID,
			User:      user,
			Search:    q,
			Limit:     limit,
			MediaType: "podcast",
		}
		podcastItems, _, err := idb.GetFilteredLibraryItems(db, optsPodcasts)
		if err == nil {
			for _, item := range podcastItems {
				podcastResults = append(podcastResults, map[string]interface{}{
					"libraryItem": item,
				})
			}
		}

		// 3. Episodes (matching in podcastEpisodes table)
		var episodeResults []map[string]interface{} = []map[string]interface{}{}
		searchTerm := "%" + q + "%"
		epRows, err := db.Query(`
			SELECT pe.id, pe.title, pe.audioFile, pe.pubDate, pe.description, pe.season, pe.episode, pe.episodeType, pe.enclosureURL, pe.publishedAt, pe.podcastId, li.id
			FROM podcastEpisodes pe
			JOIN libraryItems li ON li.mediaId = pe.podcastId AND li.mediaType = 'podcast'
			WHERE li.libraryId = ? AND (pe.title LIKE ? OR pe.description LIKE ?)
			LIMIT ?
		`, libraryID, searchTerm, searchTerm, limit)
		if err == nil {
			defer epRows.Close()
			for epRows.Next() {
				var epID, epTitle, epPodcastID, liID string
				var epAudioFile, epPubDate, epDesc, epSeason, epEpisode, epEpType, epEncURL, epPubAt sql.NullString
				if err := epRows.Scan(&epID, &epTitle, &epAudioFile, &epPubDate, &epDesc, &epSeason, &epEpisode, &epEpType, &epEncURL, &epPubAt, &epPodcastID, &liID); err == nil {
					if minItem, err := idb.GetLibraryItemMinifiedByID(db, liID); err == nil {
						epMap := map[string]interface{}{
							"id":        epID,
							"title":     epTitle,
							"audioFile": utils.NullIfEmpty(epAudioFile.String),
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
							epMap["enclosureURL"] = epEncURL.String
						}
						if epPubAt.Valid {
							epMap["publishedAt"] = epPubAt.String
						}

						minItemBytes, _ := json.Marshal(minItem)
						var minItemMap map[string]interface{}
						if json.Unmarshal(minItemBytes, &minItemMap) == nil {
							minItemMap["recentEpisode"] = epMap
							episodeResults = append(episodeResults, map[string]interface{}{
								"libraryItem": minItemMap,
							})
						}
					}
				}
			}
		}

		// 4. Authors
		var authorResults []AuthorExpandedJSON = []AuthorExpandedJSON{}
		authRows, err := db.Query(`
			SELECT id, name, lastFirst, asin, description, imagePath, createdAt, updatedAt
			FROM authors
			WHERE libraryId = ? AND (name LIKE ? OR lastFirst LIKE ?)
			LIMIT ?
		`, libraryID, searchTerm, searchTerm, limit)
		if err == nil {
			defer authRows.Close()
			for authRows.Next() {
				var id, name string
				var lastFirst, asin, description, imagePath sql.NullString
				var createdAtStr, updatedAtStr string
				if err := authRows.Scan(&id, &name, &lastFirst, &asin, &description, &imagePath, &createdAtStr, &updatedAtStr); err == nil {
					var numBooks int
					_ = db.QueryRow(`
						SELECT COUNT(DISTINCT ba.bookId)
						FROM bookAuthors ba
						JOIN libraryItems li ON li.mediaId = ba.bookId AND li.mediaType = 'book'
						WHERE ba.authorId = ? AND li.libraryId = ?
					`, id, libraryID).Scan(&numBooks)

					authorResults = append(authorResults, AuthorExpandedJSON{
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
				}
			}
		}

		// 5. Series
		var seriesResults []map[string]interface{} = []map[string]interface{}{}
		seriesRows, err := db.Query(`
			SELECT id, name, nameIgnorePrefix, description, createdAt, updatedAt
			FROM series
			WHERE libraryId = ? AND name LIKE ?
			LIMIT ?
		`, libraryID, searchTerm, limit)
		if err == nil {
			defer seriesRows.Close()
			for seriesRows.Next() {
				var id, name string
				var nameIgnorePrefix, description sql.NullString
				var createdAtStr, updatedAtStr string
				if err := seriesRows.Scan(&id, &name, &nameIgnorePrefix, &description, &createdAtStr, &updatedAtStr); err == nil {
					bookRows, err := db.Query(`
						SELECT li.id, b.coverPath, bs.sequence, li.updatedAt, li.createdAt, b.duration, b.title, b.titleIgnorePrefix
						FROM bookSeries bs
						JOIN libraryItems li ON li.mediaId = bs.bookId AND li.mediaType = 'book'
						JOIN books b ON b.id = li.mediaId
						WHERE bs.seriesId = ? AND li.libraryId = ?
					`, id, libraryID)

					books := []BookSequenceMinified{}
					if err == nil {
						for bookRows.Next() {
							var bLID, bUpdatedAtStr, bCreatedAtStr, bTitle string
							var bCoverPath, bSequence, bTitleIgnorePrefix sql.NullString
							var bDuration float64
							if err := bookRows.Scan(&bLID, &bCoverPath, &bSequence, &bUpdatedAtStr, &bCreatedAtStr, &bDuration, &bTitle, &bTitleIgnorePrefix); err == nil {
								books = append(books, BookSequenceMinified{
									ID:        bLID,
									MediaType: "book",
									UpdatedAt: idb.ParseEpochMillis(bUpdatedAtStr),
									AddedAt:   idb.ParseEpochMillis(bCreatedAtStr),
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

					seriesResults = append(seriesResults, map[string]interface{}{
						"series": map[string]interface{}{
							"id":               id,
							"name":             name,
							"nameIgnorePrefix": nameIgnorePrefix.String,
							"description":      description.String,
							"addedAt":          idb.ParseEpochMillis(createdAtStr),
							"updatedAt":        idb.ParseEpochMillis(updatedAtStr),
						},
						"books": books,
					})
				}
			}
		}

		// 6. Tags, Genres, Narrators
		tagsMap := make(map[string]int)
		genresMap := make(map[string]int)
		narratorsMap := make(map[string]int)

		rowsBooks, err := db.Query(`
			SELECT b.tags, b.genres, b.narrators
			FROM books b
			JOIN libraryItems li ON li.mediaId = b.id AND li.mediaType = 'book'
			WHERE li.libraryId = ?
		`, libraryID)
		if err == nil {
			defer rowsBooks.Close()
			for rowsBooks.Next() {
				var tagsStr, genresStr, narrStr sql.NullString
				if err := rowsBooks.Scan(&tagsStr, &genresStr, &narrStr); err == nil {
					if tagsStr.Valid && tagsStr.String != "" {
						var arr []string
						if json.Unmarshal([]byte(tagsStr.String), &arr) == nil {
							for _, v := range arr {
								if v != "" {
									tagsMap[v]++
								}
							}
						}
					}
					if genresStr.Valid && genresStr.String != "" {
						var arr []string
						if json.Unmarshal([]byte(genresStr.String), &arr) == nil {
							for _, v := range arr {
								if v != "" {
									genresMap[v]++
								}
							}
						}
					}
					if narrStr.Valid && narrStr.String != "" {
						var arr []string
						if json.Unmarshal([]byte(narrStr.String), &arr) == nil {
							for _, v := range arr {
								if v != "" {
									narratorsMap[v]++
								}
							}
						}
					}
				}
			}
		}

		rowsPodcasts, err := db.Query(`
			SELECT p.tags, p.genres
			FROM podcasts p
			JOIN libraryItems li ON li.mediaId = p.id AND li.mediaType = 'podcast'
			WHERE li.libraryId = ?
		`, libraryID)
		if err == nil {
			defer rowsPodcasts.Close()
			for rowsPodcasts.Next() {
				var tagsStr, genresStr sql.NullString
				if err := rowsPodcasts.Scan(&tagsStr, &genresStr); err == nil {
					if tagsStr.Valid && tagsStr.String != "" {
						var arr []string
						if json.Unmarshal([]byte(tagsStr.String), &arr) == nil {
							for _, v := range arr {
								if v != "" {
									tagsMap[v]++
								}
							}
						}
					}
					if genresStr.Valid && genresStr.String != "" {
						var arr []string
						if json.Unmarshal([]byte(genresStr.String), &arr) == nil {
							for _, v := range arr {
								if v != "" {
									genresMap[v]++
								}
							}
						}
					}
				}
			}
		}

		type TagResult struct {
			Name     string `json:"name"`
			NumItems int    `json:"numItems"`
		}
		var matchedTags []TagResult
		qLower := strings.ToLower(q)
		for name, count := range tagsMap {
			if strings.Contains(strings.ToLower(name), qLower) {
				matchedTags = append(matchedTags, TagResult{Name: name, NumItems: count})
			}
		}
		sort.Slice(matchedTags, func(i, j int) bool {
			if matchedTags[i].NumItems == matchedTags[j].NumItems {
				return strings.ToLower(matchedTags[i].Name) < strings.ToLower(matchedTags[j].Name)
			}
			return matchedTags[i].NumItems > matchedTags[j].NumItems
		})
		if len(matchedTags) > limit {
			matchedTags = matchedTags[:limit]
		}

		type GenreResult struct {
			Name     string `json:"name"`
			NumItems int    `json:"numItems"`
		}
		var matchedGenres []GenreResult
		for name, count := range genresMap {
			if strings.Contains(strings.ToLower(name), qLower) {
				matchedGenres = append(matchedGenres, GenreResult{Name: name, NumItems: count})
			}
		}
		sort.Slice(matchedGenres, func(i, j int) bool {
			if matchedGenres[i].NumItems == matchedGenres[j].NumItems {
				return strings.ToLower(matchedGenres[i].Name) < strings.ToLower(matchedGenres[j].Name)
			}
			return matchedGenres[i].NumItems > matchedGenres[j].NumItems
		})
		if len(matchedGenres) > limit {
			matchedGenres = matchedGenres[:limit]
		}

		type NarratorResult struct {
			Name     string `json:"name"`
			NumBooks int    `json:"numBooks"`
		}
		var matchedNarrators []NarratorResult
		for name, count := range narratorsMap {
			if strings.Contains(strings.ToLower(name), qLower) {
				matchedNarrators = append(matchedNarrators, NarratorResult{Name: name, NumBooks: count})
			}
		}
		sort.Slice(matchedNarrators, func(i, j int) bool {
			if matchedNarrators[i].NumBooks == matchedNarrators[j].NumBooks {
				return strings.ToLower(matchedNarrators[i].Name) < strings.ToLower(matchedNarrators[j].Name)
			}
			return matchedNarrators[i].NumBooks > matchedNarrators[j].NumBooks
		})
		if len(matchedNarrators) > limit {
			matchedNarrators = matchedNarrators[:limit]
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"book":      bookResults,
			"podcast":   podcastResults,
			"episodes":  episodeResults,
			"authors":   authorResults,
			"series":    seriesResults,
			"tags":      matchedTags,
			"genres":    matchedGenres,
			"narrators": matchedNarrators,
		})
	}
}
