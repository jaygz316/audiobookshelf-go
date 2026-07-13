package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"audiobookshelf/internal/core"
)

// BackupScheduleType is a custom type that can unmarshal from either a boolean or a string.
type BackupScheduleType string

func (bst *BackupScheduleType) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		*bst = ""
		return nil
	}
	if string(data) == "false" {
		*bst = ""
		return nil
	}
	if string(data) == "true" {
		*bst = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*bst = BackupScheduleType(s)
	return nil
}

func (bst BackupScheduleType) MarshalJSON() ([]byte, error) {
	if bst == "" {
		return []byte("false"), nil
	}
	return json.Marshal(string(bst))
}

// ServerSettings holds the settings stored in the database.
type ServerSettings struct {
	TokenSecret                  string             `json:"tokenSecret"`
	Language                     string             `json:"language"`
	AuthActiveAuthMethods        []string           `json:"authActiveAuthMethods"`
	AuthLoginCustomMessage       *string            `json:"authLoginCustomMessage"`
	BackupPath                   string             `json:"backupPath"`
	BackupsToKeep                int                `json:"backupsToKeep"`
	BackupSchedule               BackupScheduleType `json:"backupSchedule"`
	MetadataCoverWithItem        bool               `json:"metadataCoverWithItem"`
	MetadataMarkdownWithItem     bool               `json:"metadataMarkdownWithItem"`
	SortingIgnorePrefix          bool               `json:"sortingIgnorePrefix"`
	ScannerParseSubtitles        bool               `json:"scannerParseSubtitles"`
	ScannerFindCovers            bool               `json:"scannerFindCovers"`
	ScannerCoverProvider         string             `json:"scannerCoverProvider"`
	ScannerPreferMatchedMetadata bool               `json:"scannerPreferMatchedMetadata"`
	WatchLibraryChanges          bool               `json:"watchLibraryChanges"`
	ChromecastEnabled            bool               `json:"chromecastEnabled"`
	AllowIframe                  bool               `json:"allowIframe"`
	HomePageBookshelfView        bool               `json:"homePageBookshelfView"`
	LibraryBookshelfView         bool               `json:"libraryBookshelfView"`
	DateFormat                   string             `json:"dateFormat"`
	TimeFormat                   string             `json:"timeFormat"`
	AllowedCorsOrigins           string             `json:"allowedCorsOrigins"`
	Theme                        string             `json:"theme"`
	CustomCSS                    string             `json:"customCss"`
}

// GetServerSettings reads the server settings from the settings table.
func GetServerSettings(database *sql.DB) (*ServerSettings, error) {
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var valStr string
	err := database.QueryRow("SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
	if err != nil {
		return nil, err
	}

	var settings ServerSettings
	settings.SortingIgnorePrefix = true
	settings.ScannerParseSubtitles = true
	settings.ScannerFindCovers = true
	settings.WatchLibraryChanges = true
	settings.DateFormat = "MM/DD/YYYY"
	settings.TimeFormat = "HH:mm"
	settings.Theme = "dark"

	if err := json.Unmarshal([]byte(valStr), &settings); err != nil {
		return nil, err
	}

	// Fallback to defaults
	if len(settings.AuthActiveAuthMethods) == 0 {
		settings.AuthActiveAuthMethods = []string{"local"}
	}
	if settings.Language == "" {
		settings.Language = "en-us"
	}
	if settings.Theme == "" {
		settings.Theme = "dark"
	}

	return &settings, nil
}

// GetSortingIgnorePrefix reads sortingIgnorePrefix from server-settings.
func GetSortingIgnorePrefix(database *sql.DB) bool {
	if database == nil {
		return false
	}
	var valStr string
	err := database.QueryRow("SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
	if err != nil {
		return false
	}
	var s struct {
		SortingIgnorePrefix bool `json:"sortingIgnorePrefix"`
	}
	if err := json.Unmarshal([]byte(valStr), &s); err != nil {
		return false
	}
	return s.SortingIgnorePrefix
}

// HasRootUser checks if any user of type 'root' exists in the users table.
func HasRootUser(database *sql.DB) (bool, error) {
	if database == nil {
		return false, fmt.Errorf("database not initialized")
	}
	var count int
	err := database.QueryRow("SELECT count(*) FROM users WHERE type = 'root'").Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ParseSQLiteTime parses a SQLite timestamp string into a time.Time.
func ParseSQLiteTime(s string) (time.Time, error) {
	layouts := []string{
		"2006-01-02 15:04:05.000 +00:00",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02 15:04:05.000000 +00:00",
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("failed to parse time: %s", s)
}

// ParseEpochMillis parses a SQLite timestamp string into Unix epoch milliseconds.
func ParseEpochMillis(s string) int64 {
	t, err := ParseSQLiteTime(s)
	if err != nil {
		return 0
	}
	return t.UnixNano() / int64(time.Millisecond)
}

// userPermissions is the internal struct for parsing DB permissions JSON.
type userPermissions struct {
	Download                  *bool    `json:"download"`
	AccessExplicitContent     *bool    `json:"accessExplicitContent"`
	AccessAllLibraries        *bool    `json:"accessAllLibraries"`
	LibrariesAccessible       []string `json:"librariesAccessible"`
	Libraries                 []string `json:"libraries"`
	AccessAllTags             *bool    `json:"accessAllTags"`
	ItemTagsSelected          []string `json:"itemTagsSelected"`
	SelectedTagsNotAccessible *bool    `json:"selectedTagsNotAccessible"`
}

// ParsePermissions parses the permissions JSON string into a UserSession.
func ParsePermissions(permsStr sql.NullString, user *core.UserSession) {
	// default values:
	user.CanDownload = true
	user.CanAccessExplicitContent = false
	user.AccessAllLibraries = true
	user.LibrariesAccessible = []string{}
	user.AccessAllTags = true
	user.ItemTagsSelected = []string{}
	user.SelectedTagsNotAccessible = false

	// if it's admin or root, they have all access by default
	if user.Type == "root" || user.Type == "admin" {
		user.CanAccessExplicitContent = true
		user.AccessAllLibraries = true
		user.AccessAllTags = true
	}

	if !permsStr.Valid || permsStr.String == "" {
		return
	}

	var perms userPermissions
	if err := json.Unmarshal([]byte(permsStr.String), &perms); err != nil {
		return
	}

	if perms.Download != nil {
		user.CanDownload = *perms.Download
	}
	if perms.AccessExplicitContent != nil {
		user.CanAccessExplicitContent = *perms.AccessExplicitContent
	}
	if perms.AccessAllLibraries != nil {
		user.AccessAllLibraries = *perms.AccessAllLibraries
	}
	if perms.LibrariesAccessible != nil {
		user.LibrariesAccessible = perms.LibrariesAccessible
		if perms.AccessAllLibraries == nil {
			user.AccessAllLibraries = false
		}
	} else if perms.Libraries != nil {
		user.LibrariesAccessible = perms.Libraries
		if perms.AccessAllLibraries == nil {
			user.AccessAllLibraries = false
		}
	}
	if perms.AccessAllTags != nil {
		user.AccessAllTags = *perms.AccessAllTags
	}
	if perms.ItemTagsSelected != nil {
		user.ItemTagsSelected = perms.ItemTagsSelected
	}
	if perms.SelectedTagsNotAccessible != nil {
		user.SelectedTagsNotAccessible = *perms.SelectedTagsNotAccessible
	}
}

// GetUserByID fetches minimum info needed for authentication for a user ID.
func GetUserByID(database *sql.DB, userID string) (*core.UserSession, error) {
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var user core.UserSession
	var isActiveInt int
	var permsStr sql.NullString
	err := database.QueryRow("SELECT id, username, type, isActive, permissions FROM users WHERE id = ?", userID).
		Scan(&user.ID, &user.Username, &user.Type, &isActiveInt, &permsStr)
	if err != nil {
		return nil, err
	}
	user.IsActive = isActiveInt != 0
	ParsePermissions(permsStr, &user)
	return &user, nil
}

// GetUserByIDOrOldID fetches minimum info needed for authentication for a user ID or old user ID.
func GetUserByIDOrOldID(database *sql.DB, userID string) (*core.UserSession, error) {
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var user core.UserSession
	var isActiveInt int
	var extraDataStr string
	var permsStr sql.NullString

	err := database.QueryRow("SELECT id, username, type, isActive, extraData, permissions FROM users WHERE id = ?", userID).
		Scan(&user.ID, &user.Username, &user.Type, &isActiveInt, &extraDataStr, &permsStr)

	if err == sql.ErrNoRows {
		err = database.QueryRow("SELECT id, username, type, isActive, extraData, permissions FROM users WHERE json_extract(extraData, '$.oldUserId') = ?", userID).
			Scan(&user.ID, &user.Username, &user.Type, &isActiveInt, &extraDataStr, &permsStr)
	}

	if err != nil {
		return nil, err
	}
	user.IsActive = isActiveInt != 0
	ParsePermissions(permsStr, &user)
	return &user, nil
}

// CheckAPIKey verifies that an API key is active and not expired.
func CheckAPIKey(database *sql.DB, keyID string) (*core.UserSession, error) {
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var isActiveInt int
	var expiresAtStr sql.NullString
	var userID string

	err := database.QueryRow("SELECT isActive, expiresAt, userId FROM apiKeys WHERE id = ?", keyID).
		Scan(&isActiveInt, &expiresAtStr, &userID)
	if err != nil {
		return nil, err
	}

	if isActiveInt == 0 {
		return nil, fmt.Errorf("API key is inactive")
	}

	if expiresAtStr.Valid && expiresAtStr.String != "" {
		expiresAt, parseErr := ParseSQLiteTime(expiresAtStr.String)
		if parseErr != nil {
			return nil, fmt.Errorf("failed to parse API key expiry: %v", parseErr)
		}
		if time.Now().After(expiresAt) {
			return nil, fmt.Errorf("API key has expired")
		}
	}

	return GetUserByID(database, userID)
}

// JsonArrayToCommaString converts a JSON array of strings to a comma-separated string.
func JsonArrayToCommaString(jsonBytes []byte) string {
	if len(jsonBytes) == 0 {
		return ""
	}
	var arr []string
	if err := json.Unmarshal(jsonBytes, &arr); err != nil {
		return ""
	}
	result := ""
	for i, s := range arr {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}

// GetTokenSecret retrieves the JWT token secret from the environment or database settings.
func GetTokenSecret(database *sql.DB) string {
	if envSecret := os.Getenv("JWT_SECRET_KEY"); envSecret != "" {
		return envSecret
	}
	if database == nil {
		return ""
	}
	settings, err := GetServerSettings(database)
	if err == nil && settings != nil && settings.TokenSecret != "" {
		return settings.TokenSecret
	}
	return ""
}

// InitDB initializes the SQLite database at dbPath, creating it and bootstrapping the schema if needed.
func InitDB(dbPath string) (*sql.DB, error) {
	isNew := false
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		isNew = true
		log.Printf("[DB] Database file not found, creating new database at %s", dbPath)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode=WAL&_pragma=busy_timeout=5000", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	// Configure DB Connection Pooling for optimized SQLite WAL concurrency
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(time.Hour)
	db.SetConnMaxIdleTime(30 * time.Minute)

	if isNew {
		if err := bootstrapSchema(db); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to bootstrap schema: %w", err)
		}
		log.Printf("[DB] Schema bootstrapped successfully")
	} else {
		if err := migrateDatabase(db); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to migrate database: %w", err)
		}
	}

	return db, nil
}

func migrateDatabase(db *sql.DB) error {
	var exists int
	err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='apiKeys'").Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check if apiKeys table exists: %w", err)
	}

	if exists == 0 {
		log.Printf("[DB] Table apiKeys does not exist, creating table")
		_, err = db.Exec("CREATE TABLE apiKeys (id TEXT PRIMARY KEY, isActive INTEGER, expiresAt TEXT, userId TEXT, name TEXT, createdAt TEXT)")
		if err != nil {
			return fmt.Errorf("failed to create apiKeys table: %w", err)
		}
	}

	rows, err := db.Query("PRAGMA table_info(apiKeys)")
	if err != nil {
		return fmt.Errorf("failed to query table_info: %w", err)
	}
	defer rows.Close()

	hasName := false
	hasCreatedAt := false

	for rows.Next() {
		var cid int
		var name string
		var typeStr string
		var notnull int
		var dfltValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("failed to scan table_info row: %w", err)
		}
		if name == "name" {
			hasName = true
		}
		if name == "createdAt" {
			hasCreatedAt = true
		}
	}

	if !hasName {
		log.Printf("[DB] Migrating apiKeys table: adding name column")
		if _, err := db.Exec("ALTER TABLE apiKeys ADD COLUMN name TEXT"); err != nil {
			return fmt.Errorf("failed to add name column: %w", err)
		}
	}

	if !hasCreatedAt {
		log.Printf("[DB] Migrating apiKeys table: adding createdAt column")
		if _, err := db.Exec("ALTER TABLE apiKeys ADD COLUMN createdAt TEXT"); err != nil {
			return fmt.Errorf("failed to add createdAt column: %w", err)
		}
	}

	// Migrate playbackSessions table to include createdAt and updatedAt if missing
	var psExists int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='playbackSessions'").Scan(&psExists)
	if err != nil {
		return fmt.Errorf("failed to check if playbackSessions table exists: %w", err)
	}

	if psExists > 0 {
		psRows, err := db.Query("PRAGMA table_info(playbackSessions)")
		if err != nil {
			return fmt.Errorf("failed to query table_info for playbackSessions: %w", err)
		}
		defer psRows.Close()

		hasPsCreatedAt := false
		hasPsUpdatedAt := false

		for psRows.Next() {
			var cid int
			var name string
			var typeStr string
			var notnull int
			var dfltValue sql.NullString
			var pk int
			if err := psRows.Scan(&cid, &name, &typeStr, &notnull, &dfltValue, &pk); err != nil {
				return fmt.Errorf("failed to scan playbackSessions table_info row: %w", err)
			}
			if name == "createdAt" {
				hasPsCreatedAt = true
			}
			if name == "updatedAt" {
				hasPsUpdatedAt = true
			}
		}

		if !hasPsCreatedAt {
			log.Printf("[DB] Migrating playbackSessions table: adding createdAt column")
			if _, err := db.Exec("ALTER TABLE playbackSessions ADD COLUMN createdAt TEXT"); err != nil {
				return fmt.Errorf("failed to add createdAt column to playbackSessions: %w", err)
			}
		}

		if !hasPsUpdatedAt {
			log.Printf("[DB] Migrating playbackSessions table: adding updatedAt column")
			if _, err := db.Exec("ALTER TABLE playbackSessions ADD COLUMN updatedAt TEXT"); err != nil {
				return fmt.Errorf("failed to add updatedAt column to playbackSessions: %w", err)
			}
		}
	}

	// Migrate books table to include lockedFields if missing
	var booksExists int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='books'").Scan(&booksExists)
	if err == nil && booksExists > 0 {
		rows, err := db.Query("PRAGMA table_info(books)")
		if err == nil {
			defer rows.Close()
			hasLockedFields := false
			for rows.Next() {
				var cid int
				var name string
				var typeStr string
				var notnull int
				var dfltValue sql.NullString
				var pk int
				if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltValue, &pk); err == nil {
					if name == "lockedFields" {
						hasLockedFields = true
					}
				}
			}
			if !hasLockedFields {
				log.Printf("[DB] Migrating books table: adding lockedFields column")
				if _, err := db.Exec("ALTER TABLE books ADD COLUMN lockedFields BLOB"); err != nil {
					return fmt.Errorf("failed to add lockedFields column to books: %w", err)
				}
			}
		}
	}

	// Migrate podcasts table to include lockedFields if missing
	var podcastsExists int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='podcasts'").Scan(&podcastsExists)
	if err == nil && podcastsExists > 0 {
		rows, err := db.Query("PRAGMA table_info(podcasts)")
		if err == nil {
			defer rows.Close()
			hasLockedFields := false
			for rows.Next() {
				var cid int
				var name string
				var typeStr string
				var notnull int
				var dfltValue sql.NullString
				var pk int
				if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltValue, &pk); err == nil {
					if name == "lockedFields" {
						hasLockedFields = true
					}
				}
			}
			if !hasLockedFields {
				log.Printf("[DB] Migrating podcasts table: adding lockedFields column")
				if _, err := db.Exec("ALTER TABLE podcasts ADD COLUMN lockedFields BLOB"); err != nil {
					return fmt.Errorf("failed to add lockedFields column to podcasts: %w", err)
				}
			}
		}
	}

	// Migrate to create customMetadataProviders table if missing
	var cmpExists int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='customMetadataProviders'").Scan(&cmpExists)
	if err != nil {
		return fmt.Errorf("failed to check if customMetadataProviders table exists: %w", err)
	}

	if cmpExists == 0 {
		log.Printf("[DB] Table customMetadataProviders does not exist, creating table")
		_, err = db.Exec("CREATE TABLE customMetadataProviders (id TEXT PRIMARY KEY, name TEXT, mediaType TEXT, url TEXT, authHeaderValue TEXT, extraData TEXT, createdAt INTEGER, updatedAt INTEGER)")
		if err != nil {
			return fmt.Errorf("failed to create customMetadataProviders table: %w", err)
		}
	}

	// Migrate collections table to include isSmart and rules if missing
	var collExists int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='collections'").Scan(&collExists)
	if err == nil && collExists > 0 {
		rows, err := db.Query("PRAGMA table_info(collections)")
		if err == nil {
			defer rows.Close()
			hasIsSmart := false
			hasRules := false
			for rows.Next() {
				var cid int
				var name string
				var typeStr string
				var notnull int
				var dfltValue sql.NullString
				var pk int
				if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltValue, &pk); err == nil {
					if name == "isSmart" {
						hasIsSmart = true
					}
					if name == "rules" {
						hasRules = true
					}
				}
			}
			if !hasIsSmart {
				log.Printf("[DB] Migrating collections table: adding isSmart column")
				if _, err := db.Exec("ALTER TABLE collections ADD COLUMN isSmart INTEGER DEFAULT 0"); err != nil {
					return fmt.Errorf("failed to add isSmart column to collections: %w", err)
				}
			}
			if !hasRules {
				log.Printf("[DB] Migrating collections table: adding rules column")
				if _, err := db.Exec("ALTER TABLE collections ADD COLUMN rules TEXT"); err != nil {
					return fmt.Errorf("failed to add rules column to collections: %w", err)
				}
			}
		}
	}

	return nil
}

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
		`CREATE TABLE IF NOT EXISTS podcasts (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT, author TEXT, releaseDate TEXT, feedURL TEXT, imageURL TEXT, description TEXT, itunesPageURL TEXT, itunesId TEXT, itunesArtistId TEXT, language TEXT, podcastType TEXT, explicit INTEGER, autoDownloadEpisodes INTEGER, autoDownloadSchedule TEXT, lastEpisodeCheck TEXT, maxEpisodesToKeep INTEGER, maxNewEpisodesToDownload INTEGER, coverPath TEXT, tags BLOB, genres BLOB, numEpisodes INTEGER, lockedFields BLOB)`,
		`CREATE TABLE IF NOT EXISTS bookSeries (bookId TEXT, seriesId TEXT, sequence TEXT)`,
		`CREATE TABLE IF NOT EXISTS series (id TEXT PRIMARY KEY, libraryId TEXT, name TEXT, nameIgnorePrefix TEXT, description TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS mediaProgresses (id TEXT PRIMARY KEY, userId TEXT, mediaItemId TEXT, mediaItemType TEXT, duration REAL, currentTime REAL, isFinished INTEGER, hideFromContinueListening INTEGER, ebookLocation TEXT, ebookProgress REAL, finishedAt TEXT, extraData TEXT, podcastId TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS playbackSessions (id TEXT PRIMARY KEY, userId TEXT, mediaItemId TEXT, mediaItemType TEXT, startTime REAL, libraryId TEXT, extraData TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS podcastEpisodes (id TEXT PRIMARY KEY, podcastId TEXT, title TEXT, audioFile TEXT)`,
		`CREATE TABLE IF NOT EXISTS playlists (id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT, createdAt TEXT, updatedAt TEXT, libraryId TEXT, userId TEXT)`,
		`CREATE TABLE IF NOT EXISTS playlistMediaItems (id TEXT PRIMARY KEY, mediaItemId TEXT, mediaItemType TEXT, "order" INTEGER, createdAt TEXT, playlistId TEXT)`,
		`CREATE TABLE IF NOT EXISTS collections (id TEXT PRIMARY KEY, libraryId TEXT, name TEXT, description TEXT, createdAt TEXT, updatedAt TEXT, isSmart INTEGER DEFAULT 0, rules TEXT)`,
		`CREATE TABLE IF NOT EXISTS collectionBooks (id TEXT PRIMARY KEY, "order" INTEGER, createdAt TEXT, bookId TEXT, collectionId TEXT)`,
		`CREATE TABLE IF NOT EXISTS customMetadataProviders (id TEXT PRIMARY KEY, name TEXT, mediaType TEXT, url TEXT, authHeaderValue TEXT, extraData TEXT, createdAt INTEGER, updatedAt INTEGER)`,
		`CREATE TABLE IF NOT EXISTS authors (id TEXT PRIMARY KEY, name TEXT, lastFirst TEXT, asin TEXT, description TEXT, imagePath TEXT, createdAt TEXT, updatedAt TEXT, libraryId TEXT)`,
		`CREATE TABLE IF NOT EXISTS bookAuthors (bookId TEXT, authorId TEXT)`,
		`CREATE TABLE IF NOT EXISTS shares (id TEXT PRIMARY KEY, libraryItemId TEXT, createdBy TEXT, expiresAt TEXT, isDownloadable INTEGER, pash TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS feeds (id TEXT PRIMARY KEY, type TEXT, entityId TEXT, userId TEXT, serverAddress TEXT, createdAt TEXT, updatedAt TEXT)`,
		// Seed default server settings
		`INSERT OR IGNORE INTO settings (key, value, createdAt, updatedAt) VALUES ('server-settings', '{"sortingIgnorePrefix":true,"sortingPrefixes":["the","a"],"chromecastEnabled":false,"dateFormat":"MM/DD/YYYY","timeFormat":"HH:mm","language":"en-us","logLevel":2,"version":"2.35.1","authActiveAuthMethods":["local"],"authLoginCustomMessage":""}', datetime('now'), datetime('now'))`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("query failed (%s...): %w", q[:min(50, len(q))], err)
		}
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
