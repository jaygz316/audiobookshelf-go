# Package internal/podcast

This package manages subscribing to external podcast feeds, scheduled checks, and episode media file downloads.

## Go Signatures

```go
package podcast

import (
	"context"
	"database/sql"
)

type PodcastFeed struct {
	Title       string            `json:"title"`
	Author      string            `json:"author"`
	Description string            `json:"description"`
	Episodes    []*PodcastEpisode `json:"episodes"`
}

type PodcastEpisode struct {
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	EnclosureURL string  `json:"enclosureUrl"`
	PublishedAt  string  `json:"publishedAt"`
	Duration     float64 `json:"duration"`
}

type PodcastManager struct {
	db *sql.DB
}

// NewPodcastManager constructs a podcast subscription manager.
func NewPodcastManager(db *sql.DB) *PodcastManager

// FetchFeed parses a remote RSS feed URL.
func (m *PodcastManager) FetchFeed(ctx context.Context, url string) (*PodcastFeed, error)

// DownloadEpisode streams an episode's audio media enclosure to a local file path.
func (m *PodcastManager) DownloadEpisode(ctx context.Context, episodeURL, destPath string) error

// ScheduleRefresh initiates recurring background ticks for feed synchronization.
func (m *PodcastManager) ScheduleRefresh(ctx context.Context, cronExpression string) error
```

## Behavioral Notes
- **FetchFeed**: Handles HTTP requests with appropriate User-Agent headers, parses RSS XML, and maps enclosures to structures.
- **DownloadEpisode**: Performs chunked streaming HTTP GET requests, verifying integrity and saving the file locally.
- **ScheduleRefresh**: Configures a background runner (cron/ticker) matching settings specifications.
