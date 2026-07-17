package db

import (
	"database/sql"
	"fmt"
)

func bootstrapSchema(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS users (id TEXT PRIMARY KEY, username TEXT, email TEXT, pash TEXT, type TEXT, token TEXT, isActive INTEGER, isLocked INTEGER, lastSeen INTEGER, permissions TEXT, bookmarks TEXT, extraData TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS sessions (id TEXT PRIMARY KEY, userId TEXT, ipAddress TEXT, userAgent TEXT, refreshToken TEXT, expiresAt TEXT, lastRefreshToken TEXT, lastRefreshTokenExpiresAt TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS apiKeys (id TEXT PRIMARY KEY, isActive INTEGER, expiresAt TEXT, userId TEXT, name TEXT, createdAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS libraries (id TEXT PRIMARY KEY, name TEXT, displayOrder INTEGER, icon TEXT, mediaType TEXT, provider TEXT, lastScan TEXT, lastScanVersion TEXT, settings TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS libraryFolders (id TEXT PRIMARY KEY, path TEXT, libraryId TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS libraryItems (id TEXT PRIMARY KEY, ino TEXT, libraryId TEXT, path TEXT, relPath TEXT, isFile INTEGER, mtime TEXT, ctime TEXT, birthtime TEXT, createdAt TEXT, updatedAt TEXT, isMissing INTEGER, isInvalid INTEGER, mediaType TEXT, mediaId TEXT, size INTEGER, libraryFolderId TEXT, authorNamesFirstLast TEXT, authorNamesLastFirst TEXT, title TEXT, titleIgnorePrefix TEXT)`,
		`CREATE TABLE IF NOT EXISTS books (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT, subtitle TEXT, publishedYear TEXT, publishedDate TEXT, publisher TEXT, description TEXT, isbn TEXT, asin TEXT, language TEXT, explicit INTEGER, abridged INTEGER, coverPath TEXT, duration REAL, narrators BLOB, audioFiles BLOB, ebookFile BLOB, chapters BLOB, tags BLOB, genres BLOB, lockedFields BLOB)`,
		`CREATE TABLE IF NOT EXISTS podcasts (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT, author TEXT, releaseDate TEXT, feedURL TEXT, imageURL TEXT, description TEXT, itunesPageURL TEXT, itunesId TEXT, itunesArtistId TEXT, language TEXT, podcastType TEXT, explicit INTEGER, autoDownloadEpisodes INTEGER, autoDownloadSchedule TEXT, lastEpisodeCheck TEXT, maxEpisodesToKeep INTEGER, maxNewEpisodesToDownload INTEGER, autoDeletePlayed INTEGER DEFAULT 0, coverPath TEXT, tags BLOB, genres BLOB, numEpisodes INTEGER, lockedFields BLOB, skipIntroDuration INTEGER DEFAULT 0, skipOutroDuration INTEGER DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS bookSeries (bookId TEXT, seriesId TEXT, sequence TEXT)`,
		`CREATE TABLE IF NOT EXISTS series (id TEXT PRIMARY KEY, libraryId TEXT, name TEXT, nameIgnorePrefix TEXT, description TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS mediaProgresses (id TEXT PRIMARY KEY, userId TEXT, mediaItemId TEXT, mediaItemType TEXT, duration REAL, currentTime REAL, isFinished INTEGER, hideFromContinueListening INTEGER, ebookLocation TEXT, ebookProgress REAL, finishedAt TEXT, extraData TEXT, podcastId TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS playbackSessions (id TEXT PRIMARY KEY, userId TEXT, mediaItemId TEXT, mediaItemType TEXT, startTime REAL, libraryId TEXT, extraData TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS podcastEpisodes (id TEXT PRIMARY KEY, podcastId TEXT, title TEXT, audioFile TEXT, pubDate TEXT, description TEXT, season TEXT, episode TEXT, episodeType TEXT, enclosureURL TEXT, publishedAt TEXT, createdAt TEXT, updatedAt TEXT, imageURL TEXT)`,
		`CREATE TABLE IF NOT EXISTS playlists (id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT, createdAt TEXT, updatedAt TEXT, libraryId TEXT, userId TEXT)`,
		`CREATE TABLE IF NOT EXISTS playlistMediaItems (id TEXT PRIMARY KEY, mediaItemId TEXT, mediaItemType TEXT, "order" INTEGER, createdAt TEXT, playlistId TEXT)`,
		`CREATE TABLE IF NOT EXISTS collections (id TEXT PRIMARY KEY, libraryId TEXT, name TEXT, description TEXT, createdAt TEXT, updatedAt TEXT, isSmart INTEGER DEFAULT 0, rules TEXT)`,
		`CREATE TABLE IF NOT EXISTS collectionBooks (id TEXT PRIMARY KEY, "order" INTEGER, createdAt TEXT, bookId TEXT, collectionId TEXT)`,
		`CREATE TABLE IF NOT EXISTS customMetadataProviders (id TEXT PRIMARY KEY, name TEXT, mediaType TEXT, url TEXT, authHeaderValue TEXT, extraData TEXT, createdAt INTEGER, updatedAt INTEGER)`,
		`CREATE TABLE IF NOT EXISTS authors (id TEXT PRIMARY KEY, name TEXT, lastFirst TEXT, asin TEXT, description TEXT, imagePath TEXT, createdAt TEXT, updatedAt TEXT, libraryId TEXT)`,
		`CREATE TABLE IF NOT EXISTS bookAuthors (bookId TEXT, authorId TEXT)`,
		`CREATE TABLE IF NOT EXISTS shares (id TEXT PRIMARY KEY, libraryItemId TEXT, createdBy TEXT, expiresAt TEXT, isDownloadable INTEGER, pash TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS feeds (id TEXT PRIMARY KEY, type TEXT, entityId TEXT, userId TEXT, serverAddress TEXT, createdAt TEXT, updatedAt TEXT)`,
		// Indexes for optimization
		`CREATE INDEX IF NOT EXISTS idx_libraryItems_libraryId ON libraryItems (libraryId)`,
		`CREATE INDEX IF NOT EXISTS idx_libraryItems_mediaId_mediaType ON libraryItems (mediaId, mediaType)`,
		`CREATE INDEX IF NOT EXISTS idx_libraryItems_libraryFolderId ON libraryItems (libraryFolderId)`,
		`CREATE INDEX IF NOT EXISTS idx_libraryFolders_libraryId ON libraryFolders (libraryId)`,
		`CREATE INDEX IF NOT EXISTS idx_bookAuthors_bookId_authorId ON bookAuthors (bookId, authorId)`,
		`CREATE INDEX IF NOT EXISTS idx_bookAuthors_authorId_bookId ON bookAuthors (authorId, bookId)`,
		`CREATE INDEX IF NOT EXISTS idx_bookSeries_bookId_seriesId ON bookSeries (bookId, seriesId)`,
		`CREATE INDEX IF NOT EXISTS idx_bookSeries_seriesId_bookId ON bookSeries (seriesId, bookId)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_userId ON sessions (userId)`,
		`CREATE INDEX IF NOT EXISTS idx_mediaProgresses_userId_mediaItemId ON mediaProgresses (userId, mediaItemId)`,
		`CREATE INDEX IF NOT EXISTS idx_playbackSessions_userId ON playbackSessions (userId)`,
		`CREATE INDEX IF NOT EXISTS idx_playbackSessions_mediaItemId ON playbackSessions (mediaItemId)`,
		`CREATE INDEX IF NOT EXISTS idx_podcastEpisodes_podcastId ON podcastEpisodes (podcastId)`,
		`CREATE INDEX IF NOT EXISTS idx_playlists_userId ON playlists (userId)`,
		`CREATE INDEX IF NOT EXISTS idx_playlists_libraryId ON playlists (libraryId)`,
		`CREATE INDEX IF NOT EXISTS idx_playlistMediaItems_playlistId ON playlistMediaItems (playlistId)`,
		`CREATE INDEX IF NOT EXISTS idx_playlistMediaItems_mediaItemId ON playlistMediaItems (mediaItemId)`,
		`CREATE INDEX IF NOT EXISTS idx_collections_libraryId ON collections (libraryId)`,
		`CREATE INDEX IF NOT EXISTS idx_collectionBooks_collectionId_bookId ON collectionBooks (collectionId, bookId)`,
		`CREATE INDEX IF NOT EXISTS idx_collectionBooks_bookId_collectionId ON collectionBooks (bookId, collectionId)`,
		`CREATE INDEX IF NOT EXISTS idx_customMetadataProviders_mediaType ON customMetadataProviders (mediaType)`,
		`CREATE INDEX IF NOT EXISTS idx_authors_libraryId ON authors (libraryId)`,
		`CREATE INDEX IF NOT EXISTS idx_shares_libraryItemId ON shares (libraryItemId)`,
		`CREATE INDEX IF NOT EXISTS idx_feeds_userId ON feeds (userId)`,
		`CREATE INDEX IF NOT EXISTS idx_series_libraryId ON series (libraryId)`,
		// Seed default server settings
		`INSERT OR IGNORE INTO settings (key, value, createdAt, updatedAt) VALUES ('server-settings', '{"sortingIgnorePrefix":true,"sortingPrefixes":["the","a"],"chromecastEnabled":false,"dateFormat":"MM/DD/YYYY","timeFormat":"HH:mm","language":"en-us","logLevel":2,"version":"2.35.1","authActiveAuthMethods":["local"],"authLoginCustomMessage":""}', datetime('now'), datetime('now'))`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("query failed (%s...): %w", q[:min(50, len(q))], err)
		}
	}

	// Set database version to the latest version on fresh bootstrap
	latestVersion := len(dbMigrations)
	_, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", latestVersion))
	if err != nil {
		return fmt.Errorf("failed setting database version to latest %d: %w", latestVersion, err)
	}

	return nil
}
