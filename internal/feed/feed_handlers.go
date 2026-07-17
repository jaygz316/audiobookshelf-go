package feed

import (
	"net/http"
	"strings"
)

// ServeRSSFeed creates an HTTP handler returning the RSS XML podcast representation of a library item, playlist, collection, or series.
func (m *FeedManager) ServeRSSFeed(slug string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Reconstruct host prefix
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		host := r.Host
		if xfh := r.Header.Get("X-Forwarded-Host"); xfh != "" {
			host = xfh
		}
		hostPrefix := scheme + "://" + host

		ctx := r.Context()
		var entityID string
		var entityType string
		err := m.db.QueryRowContext(ctx, "SELECT entityId, type FROM feeds WHERE id = ?", slug).Scan(&entityID, &entityType)

		var itemID string
		if err == nil {
			itemID = entityID
		} else {
			// Fallback: check if slug itself is a valid playlist ID, collection ID, series ID, podcast ID, or book ID
			itemID = slug
			var exists int
			if m.db.QueryRowContext(ctx, "SELECT 1 FROM playlists WHERE id = ?", slug).Scan(&exists) == nil {
				entityType = "playlist"
			} else if m.db.QueryRowContext(ctx, "SELECT 1 FROM collections WHERE id = ?", slug).Scan(&exists) == nil {
				entityType = "collection"
			} else if m.db.QueryRowContext(ctx, "SELECT 1 FROM series WHERE id = ?", slug).Scan(&exists) == nil {
				entityType = "series"
			} else {
				// Must be a library item. Let's query its mediaType from libraryItems
				var mediaType string
				err := m.db.QueryRowContext(ctx, "SELECT mediaType FROM libraryItems WHERE id = ?", slug).Scan(&mediaType)
				if err == nil {
					entityType = mediaType // "book" or "podcast"
				} else {
					http.NotFound(w, r)
					return
				}
			}
		}

		// Route based on sub-path
		if strings.Contains(path, "/cover") {
			m.serveFeedCover(w, r, itemID, entityType)
			return
		}

		if strings.Contains(path, "/item/") {
			m.serveFeedItem(w, r, itemID, entityType)
			return
		}

		m.serveFeedXML(w, r, itemID, slug, hostPrefix, entityType)
	}
}
