# Package internal/playlist

This package handles user playlists and book collections.

## Go Signatures

```go
package playlist

import (
	"context"
	"database/sql"
)

type Playlist struct {
	ID        string   `json:"id"`
	UserID    string   `json:"userId"`
	Name      string   `json:"name"`
	ItemIDs   []string `json:"itemIds"`
	CreatedAt int64    `json:"createdAt"`
	UpdatedAt int64    `json:"updatedAt"`
}

type Collection struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	LibraryID    string   `json:"libraryId"`
	ItemIDs      []string `json:"itemIds"`
	DisplayOrder int      `json:"displayOrder"`
	CreatedAt    int64    `json:"createdAt"`
	UpdatedAt    int64    `json:"updatedAt"`
}

type PlaylistManager struct {
	db *sql.DB
}

func NewPlaylistManager(db *sql.DB) *PlaylistManager

func (m *PlaylistManager) CreatePlaylist(ctx context.Context, p *Playlist) error
func (m *PlaylistManager) GetPlaylist(ctx context.Context, id string) (*Playlist, error)
func (m *PlaylistManager) UpdatePlaylist(ctx context.Context, p *Playlist) error
func (m *PlaylistManager) DeletePlaylist(ctx context.Context, id string) error

func (m *PlaylistManager) CreateCollection(ctx context.Context, c *Collection) error
func (m *PlaylistManager) GetCollection(ctx context.Context, id string) (*Collection, error)
func (m *PlaylistManager) UpdateCollection(ctx context.Context, c *Collection) error
func (m *PlaylistManager) DeleteCollection(ctx context.Context, id string) error
```

## Behavioral Notes
- **PlaylistManager**: Connects directly to SQLite databases via transaction or statements. Handles playlist items mapping to relation tables (e.g. `playlistMediaItems`) and collection items to `collectionBooks`.
- **DisplayOrder**: Automatically calculates sorting sequences when inserting items into collections.
