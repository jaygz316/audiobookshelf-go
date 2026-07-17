package playlist

import (
	"database/sql"
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
