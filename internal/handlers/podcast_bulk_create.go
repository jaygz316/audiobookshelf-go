package handlers

import (
	log "audiobookshelf/internal/logger"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"audiobookshelf/internal/core"
	"audiobookshelf/internal/podcast"

	"github.com/google/uuid"
)

func handleBulkCreatePodcasts(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if globalPodcastManager == nil {
			initManagers(db)
		}
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)
		if !user.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var req struct {
			Feeds                []string `json:"feeds"`
			LibraryID            string   `json:"libraryId"`
			FolderID             string   `json:"folderId"`
			AutoDownloadEpisodes bool     `json:"autoDownloadEpisodes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}
		if len(req.Feeds) == 0 {
			http.Error(w, `{"error": "feeds parameter is required and cannot be empty"}`, http.StatusBadRequest)
			return
		}
		if req.LibraryID == "" {
			http.Error(w, `{"error": "libraryId parameter is required"}`, http.StatusBadRequest)
			return
		}

		if !user.CanAccessLibrary(req.LibraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var folderPath string
		var queryFolder string
		var err error
		if req.FolderID != "" {
			queryFolder = "SELECT path FROM libraryFolders WHERE id = ? AND libraryId = ?"
			err = db.QueryRow(queryFolder, req.FolderID, req.LibraryID).Scan(&folderPath)
		} else {
			queryFolder = "SELECT id, path FROM libraryFolders WHERE libraryId = ? LIMIT 1"
			err = db.QueryRow(queryFolder, req.LibraryID).Scan(&req.FolderID, &folderPath)
		}
		if err != nil {
			http.Error(w, `{"error": "Folder or library not found"}`, http.StatusNotFound)
			return
		}

		go func() {
			for _, feedURL := range req.Feeds {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

				feed, err := globalPodcastManager.FetchFeed(ctx, feedURL)
				if err != nil {
					log.Errorf("[BulkCreate] FetchFeed failed for %s: %v", feedURL, err)
					cancel()
					continue
				}

				title := feed.Title
				if title == "" {
					title = "Unnamed Podcast"
				}

				folderName := sanitizeFilename(title)
				podcastPath := filepath.Join(folderPath, folderName)
				if err := os.MkdirAll(podcastPath, 0755); err != nil {
					log.Errorf("[BulkCreate] MkdirAll failed for %s: %v", podcastPath, err)
					cancel()
					continue
				}

				podcastID := uuid.New().String()
				libraryItemID := uuid.New().String()

				dbModel := &PodcastDbModel{
					ID:                   podcastID,
					Title:                title,
					Author:               feed.Author,
					FeedURL:              feedURL,
					Description:          feed.Description,
					AutoDownloadEpisodes: req.AutoDownloadEpisodes,
				}

				var eps []*podcast.PodcastEpisode
				if feed != nil {
					eps = feed.Episodes
				}

				err = dbInsertPodcast(ctx, db, dbModel, libraryItemID, req.FolderID, req.LibraryID, podcastPath, eps)
				if err == nil {
					if req.AutoDownloadEpisodes {
						_ = globalPodcastManager.SyncFeed(context.Background(), podcastID)
					}
				} else {
					log.Errorf("[BulkCreate] dbInsertPodcast failed for %s: %v", feedURL, err)
				}
				cancel()
			}
		}()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}
}
