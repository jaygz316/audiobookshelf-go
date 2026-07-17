package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"sort"
	"strconv"

	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/utils"
)

// BookSequenceMinified represents a book inside the series with sequence
type BookSequenceMinified struct {
	ID        string      `json:"id"`
	MediaType string      `json:"mediaType"`
	UpdatedAt int64       `json:"updatedAt"`
	AddedAt   int64       `json:"addedAt"`
	Sequence  string      `json:"sequence"`
	Media     interface{} `json:"media"`
	Progress  interface{} `json:"userProgress,omitempty"`
}

type SeriesProgress struct {
	LibraryItemIds         []string `json:"libraryItemIds"`
	LibraryItemIdsFinished []string `json:"libraryItemIdsFinished"`
	IsFinished             bool     `json:"isFinished"`
}

// SeriesBooksJSON represents the series details with books in it
type SeriesBooksJSON struct {
	ID                   string                 `json:"id"`
	Name                 string                 `json:"name"`
	AddedAt              int64                  `json:"addedAt"`
	UpdatedAt            int64                  `json:"updatedAt"`
	NameIgnorePrefix     string                 `json:"nameIgnorePrefix"`
	NameIgnorePrefixSort string                 `json:"nameIgnorePrefixSort"`
	Type                 string                 `json:"type"`
	Books                []BookSequenceMinified `json:"books"`
	TotalDuration        float64                `json:"totalDuration"`
	Progress             *SeriesProgress        `json:"progress,omitempty"`
	LastBookAdded        int64                  `json:"-"`
	LastBookUpdated      int64                  `json:"-"`
}

func parseSequence(s string) float64 {
	if s == "" {
		return 9999.0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 9999.0
	}
	return f
}

// fetchSeriesBooksMinified retrieves minified books and user progress in a series for a library
func fetchSeriesBooksMinified(db *sql.DB, userID string, seriesID string, libraryID string) (books []BookSequenceMinified, totalDuration float64, lastBookAdded int64, lastBookUpdated int64, libraryItemIds []string, libraryItemIdsFinished []string, err error) {
	bookRows, err := db.Query(`
		SELECT li.id, b.coverPath, bs.sequence, li.updatedAt, li.createdAt, b.duration, b.title, b.titleIgnorePrefix,
		       mp.id, mp.currentTime, mp.isFinished, mp.updatedAt
		FROM bookSeries bs
		JOIN libraryItems li ON li.mediaId = bs.bookId AND li.mediaType = 'book'
		JOIN books b ON b.id = li.mediaId
		LEFT JOIN mediaProgresses mp ON mp.mediaItemId = bs.bookId AND mp.userId = ?
		WHERE bs.seriesId = ? AND li.libraryId = ?
	`, userID, seriesID, libraryID)
	if err != nil {
		return nil, 0, 0, 0, nil, nil, err
	}
	defer bookRows.Close()

	books = []BookSequenceMinified{}
	for bookRows.Next() {
		var bLID, bUpdatedAtStr, bCreatedAtStr, bTitle string
		var bCoverPath, bSequence, bTitleIgnorePrefix sql.NullString
		var bDuration float64
		var mpID, mpUpdatedAt sql.NullString
		var mpCurrentTime sql.NullFloat64
		var mpIsFinished sql.NullInt64

		if err := bookRows.Scan(&bLID, &bCoverPath, &bSequence, &bUpdatedAtStr, &bCreatedAtStr, &bDuration, &bTitle, &bTitleIgnorePrefix,
			&mpID, &mpCurrentTime, &mpIsFinished, &mpUpdatedAt); err == nil {
			bAddedAt := idb.ParseEpochMillis(bCreatedAtStr)
			bUpdatedAt := idb.ParseEpochMillis(bUpdatedAtStr)
			totalDuration += bDuration
			if bAddedAt > lastBookAdded {
				lastBookAdded = bAddedAt
			}
			if bUpdatedAt > lastBookUpdated {
				lastBookUpdated = bUpdatedAt
			}

			libraryItemIds = append(libraryItemIds, bLID)

			var userProgress interface{}
			if mpID.Valid {
				progressVal := 0.0
				if bDuration > 0 && mpCurrentTime.Valid {
					progressVal = mpCurrentTime.Float64 / bDuration
					if progressVal > 1.0 {
						progressVal = 1.0
					}
				}
				isFinished := false
				if mpIsFinished.Valid && mpIsFinished.Int64 != 0 {
					isFinished = true
					libraryItemIdsFinished = append(libraryItemIdsFinished, bLID)
				}
				currentTime := 0.0
				if mpCurrentTime.Valid {
					currentTime = mpCurrentTime.Float64
				}

				userProgress = map[string]interface{}{
					"id":            mpID.String,
					"userId":        userID,
					"libraryItemId": bLID,
					"mediaItemId":   bLID,
					"mediaItemType": "book",
					"duration":      bDuration,
					"progress":      progressVal,
					"currentTime":   currentTime,
					"isFinished":    isFinished,
					"lastUpdate":    idb.ParseTimeStr(mpUpdatedAt.String),
				}
			}

			books = append(books, BookSequenceMinified{
				ID:        bLID,
				MediaType: "book",
				UpdatedAt: bUpdatedAt,
				AddedAt:   bAddedAt,
				Sequence:  bSequence.String,
				Media: map[string]interface{}{
					"coverPath": utils.NullIfEmpty(bCoverPath.String),
					"metadata": map[string]interface{}{
						"title":             bTitle,
						"titleIgnorePrefix": bTitleIgnorePrefix.String,
					},
				},
				Progress: userProgress,
			})
		} else {
			log.Errorf("Failed to scan book row in series list: %v", err)
		}
	}

	// Sort books by sequence asc
	sort.Slice(books, func(i, j int) bool {
		return parseSequence(books[i].Sequence) < parseSequence(books[j].Sequence)
	})

	return books, totalDuration, lastBookAdded, lastBookUpdated, libraryItemIds, libraryItemIdsFinished, nil
}
