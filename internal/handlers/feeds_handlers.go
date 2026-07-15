package handlers

import (
	log "audiobookshelf/internal/logger"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

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

// handleCreateFeed creates/opens a new RSS feed.
func handleCreateFeed(db *sql.DB) http.HandlerFunc {
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

		var req CreateFeedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		if req.EntityID == "" || req.Type == "" {
			http.Error(w, `{"error": "entityId and type are required"}`, http.StatusBadRequest)
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

		// Check if a feed already exists for this entity
		var existingID, existingCreatedAt, existingUpdatedAt string
		err := db.QueryRowContext(ctx, "SELECT id, createdAt, updatedAt FROM feeds WHERE entityId = ? AND type = ?", req.EntityID, req.Type).Scan(&existingID, &existingCreatedAt, &existingUpdatedAt)
		if err == nil {
			// Already exists! Return the existing feed
			var title string
			titleQuery := ""
			switch req.Type {
			case "book", "podcast":
				titleQuery = "SELECT title FROM libraryItems WHERE id = ?"
			case "playlist":
				titleQuery = "SELECT name FROM playlists WHERE id = ?"
			case "collection":
				titleQuery = "SELECT name FROM collections WHERE id = ?"
			case "series":
				titleQuery = "SELECT name FROM series WHERE id = ?"
			}

			if titleQuery != "" {
				_ = db.QueryRowContext(ctx, titleQuery, req.EntityID).Scan(&title)
			}

			resp := FeedResponse{
				ID:            existingID,
				Type:          req.Type,
				EntityID:      req.EntityID,
				UserID:        user.ID,
				ServerAddress: hostPrefix,
				CreatedAt:     existingCreatedAt,
				UpdatedAt:     existingUpdatedAt,
				FeedURL:       fmt.Sprintf("%s/feed/%s", hostPrefix, existingID),
				Title:         title,
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		// Generate random 16-character hex slug
		bytes := make([]byte, 8)
		if _, err := rand.Read(bytes); err != nil {
			http.Error(w, `{"error": "Internal server error"}`, http.StatusInternalServerError)
			return
		}
		feedID := hex.EncodeToString(bytes)

		nowStr := time.Now().UTC().Format(time.RFC3339)

		// Insert feed
		_, err = db.ExecContext(ctx, `
			INSERT INTO feeds (id, type, entityId, userId, serverAddress, createdAt, updatedAt)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, feedID, req.Type, req.EntityID, user.ID, hostPrefix, nowStr, nowStr)
		if err != nil {
			log.Errorf("[Feeds] Failed to insert feed: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "Failed to create feed: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		// Retrieve entity title
		var title string
		titleQuery := ""
		switch req.Type {
		case "book", "podcast":
			titleQuery = "SELECT title FROM libraryItems WHERE id = ?"
		case "playlist":
			titleQuery = "SELECT name FROM playlists WHERE id = ?"
		case "collection":
			titleQuery = "SELECT name FROM collections WHERE id = ?"
		case "series":
			titleQuery = "SELECT name FROM series WHERE id = ?"
		}

		if titleQuery != "" {
			_ = db.QueryRowContext(ctx, titleQuery, req.EntityID).Scan(&title)
		}

		resp := FeedResponse{
			ID:            feedID,
			Type:          req.Type,
			EntityID:      req.EntityID,
			UserID:        user.ID,
			ServerAddress: hostPrefix,
			CreatedAt:     nowStr,
			UpdatedAt:     nowStr,
			FeedURL:       fmt.Sprintf("%s/feed/%s", hostPrefix, feedID),
			Title:         title,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
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
