package handlers

import (
	"database/sql"

	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/utils"
)

func searchAuthors(db *sql.DB, libraryID string, q string, limit int) []AuthorExpandedJSON {
	var authorResults []AuthorExpandedJSON = []AuthorExpandedJSON{}
	searchTerm := "%" + q + "%"
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
	return authorResults
}

func searchSeries(db *sql.DB, libraryID string, q string, limit int) []map[string]interface{} {
	var seriesResults []map[string]interface{} = []map[string]interface{}{}
	searchTerm := "%" + q + "%"
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
	return seriesResults
}
