package db

import (
	"database/sql"
)

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
