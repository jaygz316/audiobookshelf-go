package podcast

import (
	"database/sql"
	"os"
	"sync"

	"github.com/doyensec/safeurl"
)

// PodcastFeed represents the metadata and episode list of a podcast RSS feed.
type PodcastFeed struct {
	Title       string            `json:"title"`
	Author      string            `json:"author"`
	Description string            `json:"description"`
	Episodes    []*PodcastEpisode `json:"episodes"`
}

// PodcastEpisode represents a single episode parsed from a podcast RSS feed.
type PodcastEpisode struct {
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	EnclosureURL string  `json:"enclosureUrl"`
	PublishedAt  string  `json:"publishedAt"`
	Duration     float64 `json:"duration"`
	Season       string  `json:"season"`
	Episode      string  `json:"episode"`
	EpisodeType  string  `json:"episodeType"`
	ImageURL     string  `json:"imageUrl"`
}

// PodcastManager coordinates podcast subscriptions, feed parsing, episode downloads, and background syncing.
type PodcastManager struct {
	db      *sql.DB
	client  *safeurl.WrappedClient
	locksMu sync.Mutex
	locks   map[string]*sync.Mutex
}

// NewPodcastManager constructs a podcast subscription manager.
func NewPodcastManager(db *sql.DB) *PodcastManager {
	// PORT: Safe URL configuration builder is used to configure HTTP client against SSRF.
	builder := safeurl.GetConfigBuilder()
	if os.Getenv("BYPASS_SAFEURL") == "true" {
		builder = builder.SetAllowedIPs("127.0.0.1", "::1")
		var ports []int
		for p := 1; p <= 65535; p++ {
			ports = append(ports, p)
		}
		builder = builder.SetAllowedPorts(ports...)
	}
	config := builder.Build()
	client := safeurl.Client(config)
	return &PodcastManager{
		db:     db,
		client: client,
		locks:  make(map[string]*sync.Mutex),
	}
}

// getLock retrieves or creates a mutex for a specific podcast ID.
func (m *PodcastManager) getLock(podcastID string) *sync.Mutex {
	m.locksMu.Lock()
	defer m.locksMu.Unlock()
	if m.locks == nil {
		m.locks = make(map[string]*sync.Mutex)
	}
	lk, ok := m.locks[podcastID]
	if !ok {
		lk = &sync.Mutex{}
		m.locks[podcastID] = lk
	}
	return lk
}
