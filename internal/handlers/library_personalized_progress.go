package handlers

import (
	"database/sql"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

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
