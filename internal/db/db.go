package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"audiobookshelf/internal/core"
	log "audiobookshelf/internal/logger"
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
// If no secret is configured, it automatically generates a secure 32-byte random hex secret (256-bit entropy)
// and persists it to the server settings in the database.
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

	// Generate secure 32-byte hex secret
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Errorf("Failed to generate secure random secret: %v", err)
		return ""
	}
	secret := hex.EncodeToString(b)

	if settings == nil {
		settings = &ServerSettings{}
	}
	settings.TokenSecret = secret

	newValBytes, err := json.Marshal(settings)
	if err != nil {
		log.Errorf("Failed to marshal settings: %v", err)
		return ""
	}

	nowStr := TimeToDBStr(time.Now())
	_, err = database.Exec("INSERT INTO settings (key, value, createdAt, updatedAt) VALUES ('server-settings', ?, ?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updatedAt=excluded.updatedAt",
		string(newValBytes), nowStr, nowStr)
	if err != nil {
		log.Errorf("Failed to save secure token secret: %v", err)
		return ""
	}

	log.Info("Successfully generated and saved new secure JWT token secret to database")
	return secret
}

// InitDB initializes the SQLite database at dbPath, creating it and bootstrapping the schema if needed.
func InitDB(dbPath string) (*sql.DB, error) {
	isNew := false
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		isNew = true
		log.Infof("[DB] Database file not found, creating new database at %s", dbPath)
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

	// Configure DB Connection Pooling for optimized SQLite WAL concurrency with environment overrides
	maxOpen := 25
	maxIdle := 10
	maxLifetime := time.Hour
	maxIdleTime := 30 * time.Minute

	if val := os.Getenv("DB_MAX_OPEN_CONNS"); val != "" {
		if i, err := strconv.Atoi(val); err == nil && i > 0 {
			maxOpen = i
		}
	}
	if val := os.Getenv("DB_MAX_IDLE_CONNS"); val != "" {
		if i, err := strconv.Atoi(val); err == nil && i >= 0 {
			maxIdle = i
		}
	}
	if val := os.Getenv("DB_CONN_MAX_LIFETIME"); val != "" {
		if d, err := time.ParseDuration(val); err == nil && d > 0 {
			maxLifetime = d
		}
	}
	if val := os.Getenv("DB_CONN_MAX_IDLE_TIME"); val != "" {
		if d, err := time.ParseDuration(val); err == nil && d > 0 {
			maxIdleTime = d
		}
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(maxLifetime)
	db.SetConnMaxIdleTime(maxIdleTime)

	if isNew {
		if err := bootstrapSchema(db); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to bootstrap schema: %w", err)
		}
		log.Info("[DB] Schema bootstrapped successfully")
	} else {
		if err := migrateDatabase(db); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to migrate database: %w", err)
		}
	}

	return db, nil
}

var dbMigrations = []struct {
	version     int
	description string
	run         func(db *sql.DB) error
}{
	{
		version:     1,
		description: "Ensure apiKeys table exists and has name and createdAt columns",
		run: func(db *sql.DB) error {
			var exists int
			err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='apiKeys'").Scan(&exists)
			if err != nil {
				return err
			}
			if exists == 0 {
				_, err = db.Exec("CREATE TABLE apiKeys (id TEXT PRIMARY KEY, isActive INTEGER, expiresAt TEXT, userId TEXT, name TEXT, createdAt TEXT)")
				return err
			}
			rows, err := db.Query("PRAGMA table_info(apiKeys)")
			if err != nil {
				return err
			}
			defer rows.Close()
			hasName, hasCreatedAt := false, false
			for rows.Next() {
				var cid int
				var name, typeStr string
				var notnull int
				var dfltValue sql.NullString
				var pk int
				if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltValue, &pk); err != nil {
					return err
				}
				if name == "name" {
					hasName = true
				}
				if name == "createdAt" {
					hasCreatedAt = true
				}
			}
			if !hasName {
				if _, err := db.Exec("ALTER TABLE apiKeys ADD COLUMN name TEXT"); err != nil {
					return err
				}
			}
			if !hasCreatedAt {
				if _, err := db.Exec("ALTER TABLE apiKeys ADD COLUMN createdAt TEXT"); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		version:     2,
		description: "Ensure playbackSessions table has createdAt and updatedAt columns",
		run: func(db *sql.DB) error {
			var exists int
			err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='playbackSessions'").Scan(&exists)
			if err != nil {
				return err
			}
			if exists == 0 {
				return nil
			}
			rows, err := db.Query("PRAGMA table_info(playbackSessions)")
			if err != nil {
				return err
			}
			defer rows.Close()
			hasCreatedAt, hasUpdatedAt := false, false
			for rows.Next() {
				var cid int
				var name, typeStr string
				var notnull int
				var dfltValue sql.NullString
				var pk int
				if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltValue, &pk); err != nil {
					return err
				}
				if name == "createdAt" {
					hasCreatedAt = true
				}
				if name == "updatedAt" {
					hasUpdatedAt = true
				}
			}
			if !hasCreatedAt {
				if _, err := db.Exec("ALTER TABLE playbackSessions ADD COLUMN createdAt TEXT"); err != nil {
					return err
				}
			}
			if !hasUpdatedAt {
				if _, err := db.Exec("ALTER TABLE playbackSessions ADD COLUMN updatedAt TEXT"); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		version:     3,
		description: "Ensure books table has lockedFields column",
		run: func(db *sql.DB) error {
			var exists int
			err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='books'").Scan(&exists)
			if err != nil {
				return err
			}
			if exists == 0 {
				return nil
			}
			rows, err := db.Query("PRAGMA table_info(books)")
			if err != nil {
				return err
			}
			defer rows.Close()
			hasLockedFields := false
			for rows.Next() {
				var cid int
				var name, typeStr string
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
				if _, err := db.Exec("ALTER TABLE books ADD COLUMN lockedFields BLOB"); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		version:     4,
		description: "Ensure podcasts table has lockedFields column",
		run: func(db *sql.DB) error {
			var exists int
			err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='podcasts'").Scan(&exists)
			if err != nil {
				return err
			}
			if exists == 0 {
				return nil
			}
			rows, err := db.Query("PRAGMA table_info(podcasts)")
			if err != nil {
				return err
			}
			defer rows.Close()
			hasLockedFields := false
			for rows.Next() {
				var cid int
				var name, typeStr string
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
				if _, err := db.Exec("ALTER TABLE podcasts ADD COLUMN lockedFields BLOB"); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		version:     5,
		description: "Ensure customMetadataProviders table exists",
		run: func(db *sql.DB) error {
			var exists int
			err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='customMetadataProviders'").Scan(&exists)
			if err != nil {
				return err
			}
			if exists == 0 {
				_, err = db.Exec("CREATE TABLE customMetadataProviders (id TEXT PRIMARY KEY, name TEXT, mediaType TEXT, url TEXT, authHeaderValue TEXT, extraData TEXT, createdAt INTEGER, updatedAt INTEGER)")
				return err
			}
			return nil
		},
	},
	{
		version:     6,
		description: "Ensure collections table has isSmart and rules columns",
		run: func(db *sql.DB) error {
			var exists int
			err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='collections'").Scan(&exists)
			if err != nil {
				return err
			}
			if exists == 0 {
				return nil
			}
			rows, err := db.Query("PRAGMA table_info(collections)")
			if err != nil {
				return err
			}
			defer rows.Close()
			hasIsSmart, hasRules := false, false
			for rows.Next() {
				var cid int
				var name, typeStr string
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
				if _, err := db.Exec("ALTER TABLE collections ADD COLUMN isSmart INTEGER DEFAULT 0"); err != nil {
					return err
				}
			}
			if !hasRules {
				if _, err := db.Exec("ALTER TABLE collections ADD COLUMN rules TEXT"); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
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
	},
}

func migrateDatabase(db *sql.DB) error {
	var currentVersion int
	err := db.QueryRow("PRAGMA user_version").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("failed to read database version: %w", err)
	}

	for _, m := range dbMigrations {
		if m.version > currentVersion {
			log.Infof("[DB] Running migration version %d: %s", m.version, m.description)
			if err := m.run(db); err != nil {
				return fmt.Errorf("failed running migration %d (%s): %w", m.version, m.description, err)
			}
			_, err = db.Exec(fmt.Sprintf("PRAGMA user_version = %d", m.version))
			if err != nil {
				return fmt.Errorf("failed setting database version to %d: %w", m.version, err)
			}
			log.Infof("[DB] Successfully migrated to version %d", m.version)
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
