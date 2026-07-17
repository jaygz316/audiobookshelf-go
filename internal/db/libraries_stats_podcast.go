package db

import (
	"database/sql"
)

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
