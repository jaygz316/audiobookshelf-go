package handlers

import (
	"database/sql"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

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
