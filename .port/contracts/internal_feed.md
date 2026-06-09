# Package internal/feed

This package manages generation of XML RSS/Atom podcast feeds and OPML export files.

## Go Signatures

```go
package feed

import (
	"context"
	"database/sql"
	"net/http"
)

type FeedManager struct {
	db *sql.DB
}

// NewFeedManager constructs an XML Feed manager.
func NewFeedManager(db *sql.DB) *FeedManager

// ServeRSSFeed creates an HTTP handler returning the RSS XML podcast representation of a library item or playlist.
func (m *FeedManager) ServeRSSFeed(itemID string) http.HandlerFunc

// GenerateOPML generates an OPML XML payload mapping all podcasts inside a user's library.
func (m *FeedManager) GenerateOPML(ctx context.Context, userID, libraryID string) (string, error)
```

## Behavioral Notes
- **ServeRSSFeed**: Converts library metadata and internal episode structures into valid RSS 2.0 with iTunes podcast tags (`<itunes:summary>`, `<itunes:duration>`).
- **GenerateOPML**: Validates user access permissions to the library, scans active podcasts, and builds an OPML XML tree using standard struct marshalling.
