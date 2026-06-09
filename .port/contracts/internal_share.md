# Package internal/share

This package manages public sharing links and download tokens.

## Go Signatures

```go
package share

import (
	"context"
	"database/sql"
	"time"
)

type ShareLink struct {
	ID             string    `json:"id"`
	LibraryItemID  string    `json:"libraryItemId"`
	CreatedBy      string    `json:"createdBy"`
	ExpiresAt      time.Time `json:"expiresAt"`
	IsDownloadable bool      `json:"isDownloadable"`
	PasswordHash   string    `json:"-"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type ShareManager struct {
	db *sql.DB
}

// NewShareManager constructs a share manager.
func NewShareManager(db *sql.DB) *ShareManager

// CreateShare saves a new share link entry in the database.
func (m *ShareManager) CreateShare(ctx context.Context, s *ShareLink) error

// GetShare retrieves a share link by ID, returning nil if expired or not found.
func (m *ShareManager) GetShare(ctx context.Context, id string) (*ShareLink, error)

// ValidateSharePassword matches a plaintext password with the hashed credentials.
func (m *ShareManager) ValidateSharePassword(ctx context.Context, id, password string) (bool, error)

// DeleteShare removes the share link from database.
func (m *ShareManager) DeleteShare(ctx context.Context, id string) error
```

## Behavioral Notes
- **GetShare**: Validates expiration timescales, returning database entries only if `ExpiresAt` is in the future.
- **ValidateSharePassword**: Uses `bcrypt` comparison between stored password hash and plaintext inputs.
