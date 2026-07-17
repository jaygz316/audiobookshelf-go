package db

import (
	"database/sql"
)

var migrationV7 = migration{
	version:     7,
	description: "Create performance indexes for foreign keys and query columns",
	run: func(db *sql.DB) error {
		tableIndexes := map[string][]string{
			"libraryItems": {
				`CREATE INDEX IF NOT EXISTS idx_libraryItems_libraryId ON libraryItems (libraryId)`,
				`CREATE INDEX IF NOT EXISTS idx_libraryItems_mediaId_mediaType ON libraryItems (mediaId, mediaType)`,
				`CREATE INDEX IF NOT EXISTS idx_libraryItems_libraryFolderId ON libraryItems (libraryFolderId)`,
			},
			"libraryFolders": {
				`CREATE INDEX IF NOT EXISTS idx_libraryFolders_libraryId ON libraryFolders (libraryId)`,
			},
			"bookAuthors": {
				`CREATE INDEX IF NOT EXISTS idx_bookAuthors_bookId_authorId ON bookAuthors (bookId, authorId)`,
				`CREATE INDEX IF NOT EXISTS idx_bookAuthors_authorId_bookId ON bookAuthors (authorId, bookId)`,
			},
			"bookSeries": {
				`CREATE INDEX IF NOT EXISTS idx_bookSeries_bookId_seriesId ON bookSeries (bookId, seriesId)`,
				`CREATE INDEX IF NOT EXISTS idx_bookSeries_seriesId_bookId ON bookSeries (seriesId, bookId)`,
			},
			"sessions": {
				`CREATE INDEX IF NOT EXISTS idx_sessions_userId ON sessions (userId)`,
			},
			"mediaProgresses": {
				`CREATE INDEX IF NOT EXISTS idx_mediaProgresses_userId_mediaItemId ON mediaProgresses (userId, mediaItemId)`,
			},
			"playbackSessions": {
				`CREATE INDEX IF NOT EXISTS idx_playbackSessions_userId ON playbackSessions (userId)`,
				`CREATE INDEX IF NOT EXISTS idx_playbackSessions_mediaItemId ON playbackSessions (mediaItemId)`,
			},
			"podcastEpisodes": {
				`CREATE INDEX IF NOT EXISTS idx_podcastEpisodes_podcastId ON podcastEpisodes (podcastId)`,
			},
			"playlists": {
				`CREATE INDEX IF NOT EXISTS idx_playlists_userId ON playlists (userId)`,
				`CREATE INDEX IF NOT EXISTS idx_playlists_libraryId ON playlists (libraryId)`,
			},
			"playlistMediaItems": {
				`CREATE INDEX IF NOT EXISTS idx_playlistMediaItems_playlistId ON playlistMediaItems (playlistId)`,
				`CREATE INDEX IF NOT EXISTS idx_playlistMediaItems_mediaItemId ON playlistMediaItems (mediaItemId)`,
			},
			"collections": {
				`CREATE INDEX IF NOT EXISTS idx_collections_libraryId ON collections (libraryId)`,
			},
			"collectionBooks": {
				`CREATE INDEX IF NOT EXISTS idx_collectionBooks_collectionId_bookId ON collectionBooks (collectionId, bookId)`,
				`CREATE INDEX IF NOT EXISTS idx_collectionBooks_bookId_collectionId ON collectionBooks (bookId, collectionId)`,
			},
			"customMetadataProviders": {
				`CREATE INDEX IF NOT EXISTS idx_customMetadataProviders_mediaType ON customMetadataProviders (mediaType)`,
			},
			"authors": {
				`CREATE INDEX IF NOT EXISTS idx_authors_libraryId ON authors (libraryId)`,
			},
			"shares": {
				`CREATE INDEX IF NOT EXISTS idx_shares_libraryItemId ON shares (libraryItemId)`,
			},
			"feeds": {
				`CREATE INDEX IF NOT EXISTS idx_feeds_userId ON feeds (userId)`,
			},
			"series": {
				`CREATE INDEX IF NOT EXISTS idx_series_libraryId ON series (libraryId)`,
			},
		}
		for tbl, idxs := range tableIndexes {
			var count int
			err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&count)
			if err != nil {
				return err
			}
			if count > 0 {
				for _, index := range idxs {
					if _, err := db.Exec(index); err != nil {
						return err
					}
				}
			}
		}
		return nil
	},
}
