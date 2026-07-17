package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
)

type FeedResponse struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	EntityID      string `json:"entityId"`
	UserID        string `json:"userId"`
	ServerAddress string `json:"serverAddress"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
	FeedURL       string `json:"feedUrl"`
	Title         string `json:"title"`
}

type CreateFeedRequest struct {
	EntityID string `json:"entityId"`
	Type     string `json:"type"`
}

// handleGetFeeds returns the list of active feeds.
func handleGetFeeds(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)
		if user.Type != "root" && user.Type != "admin" && !user.CanAccessRss {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		ctx := r.Context()

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

		query := `
			SELECT f.id, f.type, f.entityId, f.userId, f.serverAddress, f.createdAt, f.updatedAt,
			       COALESCE(li.title, pl.name, c.name, s.name, '') as title
			FROM feeds f
			LEFT JOIN libraryItems li ON f.entityId = li.id AND f.type IN ('book', 'podcast')
			LEFT JOIN playlists pl ON f.entityId = pl.id AND f.type = 'playlist'
			LEFT JOIN collections c ON f.entityId = c.id AND f.type = 'collection'
			LEFT JOIN series s ON f.entityId = s.id AND f.type = 'series'
		`

		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			log.Errorf("[Feeds] Failed to query feeds: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "Failed to query feeds: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		feeds := []FeedResponse{}
		for rows.Next() {
			var f FeedResponse
			if err := rows.Scan(&f.ID, &f.Type, &f.EntityID, &f.UserID, &f.ServerAddress, &f.CreatedAt, &f.UpdatedAt, &f.Title); err != nil {
				log.Errorf("[Feeds] Failed to scan feed row: %v", err)
				continue
			}
			f.FeedURL = fmt.Sprintf("%s/feed/%s", hostPrefix, f.ID)
			feeds = append(feeds, f)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string][]FeedResponse{"feeds": feeds})
	}
}

// handleDeleteFeed deletes/closes an RSS feed.
func handleDeleteFeed(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)
		if user.Type != "root" && user.Type != "admin" && !user.CanAccessRss {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		id := trimPathPrefix(r.URL.Path, "/api/feeds/")
		id = strings.TrimSuffix(id, "/")
		if id == "" {
			http.Error(w, `{"error": "ID is required"}`, http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		res, err := db.ExecContext(ctx, "DELETE FROM feeds WHERE id = ?", id)
		if err != nil {
			log.Errorf("[Feeds] Failed to delete feed: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "Failed to delete feed: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		rowsAffected, _ := res.RowsAffected()
		if rowsAffected == 0 {
			http.Error(w, `{"error": "Feed not found"}`, http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
