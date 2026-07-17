package handlers

import (
	"database/sql"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

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
