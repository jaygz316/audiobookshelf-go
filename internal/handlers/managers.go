package handlers

import (
	"database/sql"
	"log"
	"sync"

	"audiobookshelf/internal/auth"
	"audiobookshelf/internal/feed"
	"audiobookshelf/internal/finders"
	"audiobookshelf/internal/hls"
	"audiobookshelf/internal/playlist"
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
	globalFinder          *finders.Finder
	globalDB              *sql.DB
)

var managersMu sync.Mutex

func initManagers(db *sql.DB) {
	managersMu.Lock()
	defer managersMu.Unlock()

	if db == nil {
		log.Println("[Warning] initManagers: database connection is nil. Deferring initialization.")
		return
	}

	if globalFinder == nil {
		globalFinder = finders.NewFinder([]providers.Provider{
			&providers.AudibleProvider{},
			&providers.AudnexusProvider{},
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
		globalFeedManager = feed.NewFeedManager(db)
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
	globalFeedManager = feed.NewFeedManager(db)
}
