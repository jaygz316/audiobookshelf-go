package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"sync"

	"audiobookshelf/internal/auth"
	"audiobookshelf/internal/feed"
	"audiobookshelf/internal/finders"
	"audiobookshelf/internal/hls"
	"audiobookshelf/internal/playlist"
	"audiobookshelf/internal/podcast"
	"audiobookshelf/internal/providers"
	"audiobookshelf/internal/share"
)

var (
	streamManager       = hls.NewStreamManager()
	globalOIDCHandler   *auth.OIDCHandler
	globalOIDCHandlerMu sync.RWMutex

	globalShareManager    *share.ShareManager
	globalPlaylistManager *playlist.PlaylistManager
	globalFeedManager     *feed.FeedManager
	globalPodcastManager  *podcast.PodcastManager
	globalFinder          *finders.Finder
	globalDB              *sql.DB
)

var globalDBMu sync.RWMutex

func GetGlobalDB() *sql.DB {
	globalDBMu.RLock()
	defer globalDBMu.RUnlock()
	return globalDB
}

func SetGlobalDB(db *sql.DB) {
	globalDBMu.Lock()
	globalDB = db
	globalDBMu.Unlock()
}

var managersMu sync.Mutex

func initManagers(db *sql.DB) {
	managersMu.Lock()
	defer managersMu.Unlock()

	if db == nil {
		log.Println("[Warning] initManagers: database connection is nil. Deferring initialization.")
		return
	}

	if globalFinder == nil {
		globalFinder = finders.NewFinder(db, []providers.Provider{
			&providers.AudibleProvider{},
			&providers.AudnexusProvider{},
			&providers.FantLabProvider{},
			&providers.GoogleBooksProvider{},
			&providers.ITunesProvider{},
			&providers.OpenLibraryProvider{},
		})
	}

	if globalShareManager == nil {
		globalShareManager = share.NewShareManager(db)
	}
	if globalPlaylistManager == nil {
		globalPlaylistManager = playlist.NewPlaylistManager(db)
	}
	if globalFeedManager == nil {
		globalFeedManager = feed.NewFeedManager(db, MetadataPath)
	}
	if globalPodcastManager == nil {
		globalPodcastManager = podcast.NewPodcastManager(db)
		podcast.InitQueueManager(db, globalPodcastManager)
	}
}

func reinitManagers(db *sql.DB) {
	managersMu.Lock()
	defer managersMu.Unlock()

	if db == nil {
		log.Println("[Warning] reinitManagers: database connection is nil.")
		return
	}

	log.Println("[Info] reinitManagers: updating database connection for managers.")
	globalShareManager = share.NewShareManager(db)
	globalPlaylistManager = playlist.NewPlaylistManager(db)
	globalFeedManager = feed.NewFeedManager(db, MetadataPath)
	globalPodcastManager = podcast.NewPodcastManager(db)
	podcast.InitQueueManager(db, globalPodcastManager)
	globalFinder = finders.NewFinder(db, []providers.Provider{
		&providers.AudibleProvider{},
		&providers.AudnexusProvider{},
		&providers.FantLabProvider{},
		&providers.GoogleBooksProvider{},
		&providers.ITunesProvider{},
		&providers.OpenLibraryProvider{},
	})
}

// ShutdownStreamManager shuts down all active transcoding sessions to prevent orphaned FFmpeg processes.
func ShutdownStreamManager() {
	if streamManager != nil {
		streamManager.Close()
	}
}
