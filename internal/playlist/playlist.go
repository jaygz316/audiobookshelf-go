package playlist

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Playlist represents a user-created media playlist containing multiple items.
type Playlist struct {
	ID        string   `json:"id"`
	UserID    string   `json:"userId"`
	Name      string   `json:"name"`
	ItemIDs   []string `json:"itemIds"`
	CreatedAt int64    `json:"createdAt"`
	UpdatedAt int64    `json:"updatedAt"`
}

// Collection represents a library-specific collection of books.
type Collection struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	LibraryID    string   `json:"libraryId"`
	ItemIDs      []string `json:"itemIds"`
	DisplayOrder int      `json:"displayOrder"`
	IsSmart      bool     `json:"isSmart"`
	Rules        string   `json:"rules"`
	CreatedAt    int64    `json:"createdAt"`
	UpdatedAt    int64    `json:"updatedAt"`
}

// PlaylistManager handles database CRUD operations for playlists and collections.
type PlaylistManager struct {
	db *sql.DB
}

// NewPlaylistManager creates a new instance of PlaylistManager using the provided SQL database connection.
func NewPlaylistManager(db *sql.DB) *PlaylistManager {
	return &PlaylistManager{
		db: db,
	}
}

// Helper: parse SQLite times
func parseSQLiteTime(s string) (time.Time, error) {
	layouts := []string{
		"2006-01-02 15:04:05.000 +00:00",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02 15:04:05.000000 +00:00",
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("failed to parse sqlite time %q", s)
}

func msToTimeStr(ms int64) string {
	t := time.Unix(ms/1000, (ms%1000)*1000000).UTC()
	return t.Format("2006-01-02 15:04:05.000")
}

func parseMsFromDBStr(s string) int64 {
	t, err := parseSQLiteTime(s)
	if err != nil {
		return 0
	}
	return t.UnixNano() / int64(time.Millisecond)
}

// Helper: check if a table contains a column dynamically
func hasColumn(ctx context.Context, db *sql.DB, tableName, columnName string) bool {
	query := fmt.Sprintf("PRAGMA table_info(%s)", tableName)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name string
		var typeStr string
		var notnull int
		var dfltVal interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltVal, &pk); err == nil {
			if strings.EqualFold(name, columnName) {
				return true
			}
		}
	}
	if err := rows.Err(); err != nil {
		return false
	}
	return false
}

// Helper: query media item types from books or podcastEpisodes
func (m *PlaylistManager) getMediaItemTypes(ctx context.Context, tx *sql.Tx, itemIDs []string) (map[string]string, error) {
	if len(itemIDs) == 0 {
		return make(map[string]string), nil
	}

	placeholders := make([]string, len(itemIDs))
	args := make([]interface{}, len(itemIDs))
	for i, id := range itemIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	inClause := strings.Join(placeholders, ",")
	query := fmt.Sprintf(
		"SELECT id, 'book' FROM books WHERE id IN (%s) UNION ALL SELECT id, 'podcastEpisode' FROM podcastEpisodes WHERE id IN (%s)",
		inClause, inClause,
	)
	doubleArgs := append(args, args...)

	rows, err := tx.QueryContext(ctx, query, doubleArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to query media item types: %w", err)
	}
	defer rows.Close()

	typeMap := make(map[string]string)
	for rows.Next() {
		var id, itemType string
		if err := rows.Scan(&id, &itemType); err != nil {
			return nil, fmt.Errorf("failed to scan media item type: %w", err)
		}
		typeMap[id] = itemType
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during media item types iteration: %w", err)
	}
	return typeMap, nil
}

// CreatePlaylist creates a new playlist record in the database along with its media items.
func (m *PlaylistManager) CreatePlaylist(ctx context.Context, p *Playlist) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	if p.CreatedAt == 0 {
		p.CreatedAt = time.Now().UnixNano() / int64(time.Millisecond)
	}
	if p.UpdatedAt == 0 {
		p.UpdatedAt = p.CreatedAt
	}

	createdAtStr := msToTimeStr(p.CreatedAt)
	updatedAtStr := msToTimeStr(p.UpdatedAt)

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert into playlists. libraryId is NULL, description is NULL.
	// PORT: libraryId and description are not exposed in Playlist struct so we insert NULL for libraryId and NULL for description.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO playlists (id, name, description, createdAt, updatedAt, libraryId, userId)
		VALUES (?, ?, NULL, ?, ?, NULL, ?)`,
		p.ID, p.Name, createdAtStr, updatedAtStr, p.UserID)
	if err != nil {
		return fmt.Errorf("failed to insert playlist: %w", err)
	}

	// Lookup media item types
	typeMap, err := m.getMediaItemTypes(ctx, tx, p.ItemIDs)
	if err != nil {
		return fmt.Errorf("failed to get media item types: %w", err)
	}

	// Insert items
	for i, itemID := range p.ItemIDs {
		itemType := typeMap[itemID]
		if itemType == "" {
			itemType = "book" // Default to book if not found
		}
		itemUUID := uuid.New().String()
		order := i + 1

		_, err = tx.ExecContext(ctx, `
			INSERT INTO playlistMediaItems (id, mediaItemId, mediaItemType, "order", createdAt, playlistId)
			VALUES (?, ?, ?, ?, ?, ?)`,
			itemUUID, itemID, itemType, order, createdAtStr, p.ID)
		if err != nil {
			return fmt.Errorf("failed to insert playlist media item: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// GetPlaylist retrieves a playlist and its associated media item IDs by ID from the database.
func (m *PlaylistManager) GetPlaylist(ctx context.Context, id string) (*Playlist, error) {
	var userID, name, createdAtStr, updatedAtStr string
	err := m.db.QueryRowContext(ctx, `
		SELECT userId, name, createdAt, updatedAt FROM playlists WHERE id = ?`, id).
		Scan(&userID, &name, &createdAtStr, &updatedAtStr)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	} else if err != nil {
		return nil, fmt.Errorf("failed to query playlist: %w", err)
	}

	p := &Playlist{
		ID:        id,
		UserID:    userID,
		Name:      name,
		CreatedAt: parseMsFromDBStr(createdAtStr),
		UpdatedAt: parseMsFromDBStr(updatedAtStr),
		ItemIDs:   []string{},
	}

	rows, err := m.db.QueryContext(ctx, `
		SELECT mediaItemId FROM playlistMediaItems WHERE playlistId = ? ORDER BY "order" ASC`, id)
	if err != nil {
		return nil, fmt.Errorf("failed to query playlist media items: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var itemID string
		if err := rows.Scan(&itemID); err != nil {
			return nil, fmt.Errorf("failed to scan playlist media item: %w", err)
		}
		p.ItemIDs = append(p.ItemIDs, itemID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during playlist media items iteration: %w", err)
	}

	return p, nil
}

// UpdatePlaylist updates an existing playlist's details and replaces its items with a new set of media items.
func (m *PlaylistManager) UpdatePlaylist(ctx context.Context, p *Playlist) error {
	p.UpdatedAt = time.Now().UnixNano() / int64(time.Millisecond)
	updatedAtStr := msToTimeStr(p.UpdatedAt)

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Update playlist name and updatedAt
	_, err = tx.ExecContext(ctx, `
		UPDATE playlists SET name = ?, updatedAt = ? WHERE id = ?`,
		p.Name, updatedAtStr, p.ID)
	if err != nil {
		return fmt.Errorf("failed to update playlist: %w", err)
	}

	// Delete old items
	_, err = tx.ExecContext(ctx, `
		DELETE FROM playlistMediaItems WHERE playlistId = ?`, p.ID)
	if err != nil {
		return fmt.Errorf("failed to delete playlist media items: %w", err)
	}

	// Lookup media item types
	typeMap, err := m.getMediaItemTypes(ctx, tx, p.ItemIDs)
	if err != nil {
		return fmt.Errorf("failed to get media item types: %w", err)
	}

	// Insert items with sequential order
	for i, itemID := range p.ItemIDs {
		itemType := typeMap[itemID]
		if itemType == "" {
			itemType = "book"
		}
		itemUUID := uuid.New().String()
		order := i + 1

		_, err = tx.ExecContext(ctx, `
			INSERT INTO playlistMediaItems (id, mediaItemId, mediaItemType, "order", createdAt, playlistId)
			VALUES (?, ?, ?, ?, ?, ?)`,
			itemUUID, itemID, itemType, order, updatedAtStr, p.ID)
		if err != nil {
			return fmt.Errorf("failed to insert playlist media item: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// DeletePlaylist removes a playlist and its associated items from the database.
func (m *PlaylistManager) DeletePlaylist(ctx context.Context, id string) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete playlist items
	_, err = tx.ExecContext(ctx, `
		DELETE FROM playlistMediaItems WHERE playlistId = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete playlist media items: %w", err)
	}

	// Delete playlist itself
	_, err = tx.ExecContext(ctx, `
		DELETE FROM playlists WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete playlist: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// CreateCollection creates a new collection record in the database.
func (m *PlaylistManager) CreateCollection(ctx context.Context, c *Collection) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	if c.CreatedAt == 0 {
		c.CreatedAt = time.Now().UnixNano() / int64(time.Millisecond)
	}
	if c.UpdatedAt == 0 {
		c.UpdatedAt = c.CreatedAt
	}

	createdAtStr := msToTimeStr(c.CreatedAt)
	updatedAtStr := msToTimeStr(c.UpdatedAt)

	// Check if "displayOrder" column exists in "collections" table dynamically
	hasDisplayOrder := hasColumn(ctx, m.db, "collections", "displayOrder")

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	isSmartInt := 0
	if c.IsSmart {
		isSmartInt = 1
	}

	if hasDisplayOrder {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO collections (id, name, description, createdAt, updatedAt, libraryId, displayOrder, isSmart, rules)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			c.ID, c.Name, c.Description, createdAtStr, updatedAtStr, c.LibraryID, c.DisplayOrder, isSmartInt, c.Rules)
	} else {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO collections (id, name, description, createdAt, updatedAt, libraryId, isSmart, rules)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			c.ID, c.Name, c.Description, createdAtStr, updatedAtStr, c.LibraryID, isSmartInt, c.Rules)
	}
	if err != nil {
		return fmt.Errorf("failed to insert collection: %w", err)
	}

	// Insert items into collectionBooks if not smart
	// PORT: Automatically compute/reorder display order for collectionBooks by indexing them starting from 1.
	if !c.IsSmart {
		for i, itemID := range c.ItemIDs {
			cbUUID := uuid.New().String()
			order := i + 1

			_, err = tx.ExecContext(ctx, `
				INSERT INTO collectionBooks (id, "order", createdAt, bookId, collectionId)
				VALUES (?, ?, ?, ?, ?)`,
				cbUUID, order, createdAtStr, itemID, c.ID)
			if err != nil {
				return fmt.Errorf("failed to insert collection book: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// SmartCollectionRules defines the matching rules for a Smart Collection.
type SmartCollectionRules struct {
	Genres         []string `json:"genres,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	Authors        []string `json:"authors,omitempty"`
	Series         []string `json:"series,omitempty"`
	Narrators      []string `json:"narrators,omitempty"`
	PublishedYears []string `json:"publishedYears,omitempty"`
	Search         string   `json:"search,omitempty"`
}

// ResolveSmartCollectionItems dynamically queries library items matching smart rules.
func (m *PlaylistManager) ResolveSmartCollectionItems(ctx context.Context, libraryID string, rulesJSON string) ([]string, error) {
	if rulesJSON == "" {
		return []string{}, nil
	}

	var rules SmartCollectionRules
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		return nil, fmt.Errorf("failed to unmarshal rules: %w", err)
	}

	query := `
		SELECT li.mediaId
		FROM libraryItems li
		JOIN books b ON li.mediaId = b.id AND li.mediaType = 'book'
		WHERE li.libraryId = ? AND li.isMissing = 0 AND li.isInvalid = 0
	`
	args := []interface{}{libraryID}

	if len(rules.Genres) > 0 {
		var placeholders []string
		for _, genre := range rules.Genres {
			if g := strings.TrimSpace(genre); g != "" {
				placeholders = append(placeholders, "?")
				args = append(args, g)
			}
		}
		if len(placeholders) > 0 {
			query += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM json_each(b.genres) WHERE json_valid(b.genres) AND json_each.value COLLATE NOCASE IN (%s))", strings.Join(placeholders, ","))
		}
	}

	if len(rules.Tags) > 0 {
		var placeholders []string
		for _, tag := range rules.Tags {
			if t := strings.TrimSpace(tag); t != "" {
				placeholders = append(placeholders, "?")
				args = append(args, t)
			}
		}
		if len(placeholders) > 0 {
			query += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM json_each(b.tags) WHERE json_valid(b.tags) AND json_each.value COLLATE NOCASE IN (%s))", strings.Join(placeholders, ","))
		}
	}

	if len(rules.Authors) > 0 {
		var placeholders []string
		for _, author := range rules.Authors {
			if a := strings.TrimSpace(author); a != "" {
				placeholders = append(placeholders, "?")
				args = append(args, a)
			}
		}
		if len(placeholders) > 0 {
			query += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM bookAuthors ba JOIN authors a ON ba.authorId = a.id WHERE ba.bookId = b.id AND a.name COLLATE NOCASE IN (%s))", strings.Join(placeholders, ","))
		}
	}

	if len(rules.Series) > 0 {
		var placeholders []string
		for _, ser := range rules.Series {
			if s := strings.TrimSpace(ser); s != "" {
				placeholders = append(placeholders, "?")
				args = append(args, s)
			}
		}
		if len(placeholders) > 0 {
			query += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM bookSeries bs JOIN series s ON bs.seriesId = s.id WHERE bs.bookId = b.id AND s.name COLLATE NOCASE IN (%s))", strings.Join(placeholders, ","))
		}
	}

	if len(rules.Narrators) > 0 {
		var placeholders []string
		for _, narrator := range rules.Narrators {
			if n := strings.TrimSpace(narrator); n != "" {
				placeholders = append(placeholders, "?")
				args = append(args, n)
			}
		}
		if len(placeholders) > 0 {
			query += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM json_each(b.narrators) WHERE json_valid(b.narrators) AND json_each.value COLLATE NOCASE IN (%s))", strings.Join(placeholders, ","))
		}
	}

	if len(rules.PublishedYears) > 0 {
		var placeholders []string
		for _, year := range rules.PublishedYears {
			if y := strings.TrimSpace(year); y != "" {
				placeholders = append(placeholders, "?")
				args = append(args, y)
			}
		}
		if len(placeholders) > 0 {
			query += fmt.Sprintf(" AND b.publishedYear COLLATE NOCASE IN (%s)", strings.Join(placeholders, ","))
		}
	}

	if rules.Search != "" {
		searchTerm := "%" + rules.Search + "%"
		query += ` AND (
			b.title LIKE ? OR 
			b.subtitle LIKE ? OR 
			b.description LIKE ? OR 
			EXISTS (SELECT 1 FROM bookAuthors ba JOIN authors a ON ba.authorId = a.id WHERE ba.bookId = b.id AND a.name LIKE ?)
		)`
		args = append(args, searchTerm, searchTerm, searchTerm, searchTerm)
	}

	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query smart collection books: %w", err)
	}
	defer rows.Close()

	var bookIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan book id: %w", err)
		}
		bookIDs = append(bookIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error in rows iteration: %w", err)
	}

	return bookIDs, nil
}

// GetCollection retrieves a collection and its associated book IDs by ID from the database.
func (m *PlaylistManager) GetCollection(ctx context.Context, id string) (*Collection, error) {
	var name, description, libraryID, createdAtStr, updatedAtStr string
	var displayOrder int
	var isSmart sql.NullInt64
	var rules sql.NullString

	hasDisplayOrder := hasColumn(ctx, m.db, "collections", "displayOrder")

	var err error
	if hasDisplayOrder {
		err = m.db.QueryRowContext(ctx, `
			SELECT name, description, libraryId, displayOrder, createdAt, updatedAt, isSmart, rules FROM collections WHERE id = ?`, id).
			Scan(&name, &description, &libraryID, &displayOrder, &createdAtStr, &updatedAtStr, &isSmart, &rules)
	} else {
		err = m.db.QueryRowContext(ctx, `
			SELECT name, description, libraryId, createdAt, updatedAt, isSmart, rules FROM collections WHERE id = ?`, id).
			Scan(&name, &description, &libraryID, &createdAtStr, &updatedAtStr, &isSmart, &rules)
	}

	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	} else if err != nil {
		return nil, fmt.Errorf("failed to query collection: %w", err)
	}

	c := &Collection{
		ID:           id,
		Name:         name,
		Description:  description,
		LibraryID:    libraryID,
		DisplayOrder: displayOrder,
		IsSmart:      isSmart.Int64 != 0,
		Rules:        rules.String,
		CreatedAt:    parseMsFromDBStr(createdAtStr),
		UpdatedAt:    parseMsFromDBStr(updatedAtStr),
		ItemIDs:      []string{},
	}

	if c.IsSmart {
		dynamicIDs, err := m.ResolveSmartCollectionItems(ctx, c.LibraryID, c.Rules)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve smart collection items: %w", err)
		}
		c.ItemIDs = dynamicIDs
	} else {
		// Get collection books in display order
		rows, err := m.db.QueryContext(ctx, `
			SELECT bookId FROM collectionBooks WHERE collectionId = ? ORDER BY "order" ASC`, id)
		if err != nil {
			return nil, fmt.Errorf("failed to query collection books: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var bookID string
			if err := rows.Scan(&bookID); err != nil {
				return nil, fmt.Errorf("failed to scan collection book: %w", err)
			}
			c.ItemIDs = append(c.ItemIDs, bookID)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("error during collection books iteration: %w", err)
		}
	}

	return c, nil
}

// UpdateCollection updates an existing collection's details and replaces its books list.
func (m *PlaylistManager) UpdateCollection(ctx context.Context, c *Collection) error {
	c.UpdatedAt = time.Now().UnixNano() / int64(time.Millisecond)
	updatedAtStr := msToTimeStr(c.UpdatedAt)

	hasDisplayOrder := hasColumn(ctx, m.db, "collections", "displayOrder")

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	isSmartInt := 0
	if c.IsSmart {
		isSmartInt = 1
	}

	if hasDisplayOrder {
		_, err = tx.ExecContext(ctx, `
			UPDATE collections SET name = ?, description = ?, libraryId = ?, displayOrder = ?, isSmart = ?, rules = ?, updatedAt = ? WHERE id = ?`,
			c.Name, c.Description, c.LibraryID, c.DisplayOrder, isSmartInt, c.Rules, updatedAtStr, c.ID)
	} else {
		_, err = tx.ExecContext(ctx, `
			UPDATE collections SET name = ?, description = ?, libraryId = ?, isSmart = ?, rules = ?, updatedAt = ? WHERE id = ?`,
			c.Name, c.Description, c.LibraryID, isSmartInt, c.Rules, updatedAtStr, c.ID)
	}
	if err != nil {
		return fmt.Errorf("failed to update collection: %w", err)
	}

	// Delete old items
	_, err = tx.ExecContext(ctx, `
		DELETE FROM collectionBooks WHERE collectionId = ?`, c.ID)
	if err != nil {
		return fmt.Errorf("failed to delete collection books: %w", err)
	}

	// Insert items into collectionBooks if not smart
	// PORT: Automatically compute/reorder display order for collectionBooks by indexing them starting from 1.
	if !c.IsSmart {
		for i, itemID := range c.ItemIDs {
			cbUUID := uuid.New().String()
			order := i + 1

			_, err = tx.ExecContext(ctx, `
				INSERT INTO collectionBooks (id, "order", createdAt, bookId, collectionId)
				VALUES (?, ?, ?, ?, ?)`,
				cbUUID, order, updatedAtStr, itemID, c.ID)
			if err != nil {
				return fmt.Errorf("failed to insert collection book: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// DeleteCollection removes a collection and its book relations from the database.
func (m *PlaylistManager) DeleteCollection(ctx context.Context, id string) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete collection books
	_, err = tx.ExecContext(ctx, `
		DELETE FROM collectionBooks WHERE collectionId = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete collection books: %w", err)
	}

	// Delete collection itself
	_, err = tx.ExecContext(ctx, `
		DELETE FROM collections WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete collection: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}
