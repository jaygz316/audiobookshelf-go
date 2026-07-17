package handlers

import (
	log "audiobookshelf/internal/logger"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"audiobookshelf/internal/core"
)

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
