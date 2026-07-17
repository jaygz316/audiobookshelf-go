package handlers

import (
	log "audiobookshelf/internal/logger"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/podcast"

	"github.com/google/uuid"
)

type CreatePodcastRequest struct {
	LibraryID            string `json:"libraryId"`
	FolderID             string `json:"folderId"`
	FeedURL              string `json:"feedUrl"`
	AutoDownloadEpisodes bool   `json:"autoDownloadEpisodes"`
	Metadata             struct {
		Title       string   `json:"title"`
		Author      string   `json:"author"`
		Description string   `json:"description"`
		FeedURL     string   `json:"feedUrl"`
		Language    string   `json:"language"`
		Explicit    bool     `json:"explicit"`
		Genres      []string `json:"genres"`
	} `json:"metadata"`
}

func handleCreatePodcast(db *sql.DB) http.HandlerFunc {
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

		var req CreatePodcastRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
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

		feedURL := req.FeedURL
		if feedURL == "" {
			feedURL = req.Metadata.FeedURL
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

		var feed *podcast.PodcastFeed
		if feedURL != "" {
			feed, err = globalPodcastManager.FetchFeed(r.Context(), feedURL)
			if err != nil {
				log.Errorf("[CreatePodcast] FetchFeed failed: %v", err)
				http.Error(w, fmt.Sprintf(`{"error": "Failed to fetch podcast feed: %s"}`, err.Error()), http.StatusBadRequest)
				return
			}
		}

		title := req.Metadata.Title
		author := req.Metadata.Author
		description := req.Metadata.Description
		language := req.Metadata.Language
		explicit := req.Metadata.Explicit
		genres := req.Metadata.Genres

		if feed != nil {
			if title == "" {
				title = feed.Title
			}
			if author == "" {
				author = feed.Author
			}
			if description == "" {
				description = feed.Description
			}
		}

		if title == "" {
			title = "Unnamed Podcast"
		}

		folderName := sanitizeFilename(title)
		podcastPath := filepath.Join(folderPath, folderName)
		if err := os.MkdirAll(podcastPath, 0755); err != nil {
			log.Errorf("[CreatePodcast] MkdirAll failed for path %s: %v", podcastPath, err)
			http.Error(w, `{"error": "Failed to create podcast folder on disk"}`, http.StatusInternalServerError)
			return
		}

		podcastID := uuid.New().String()
		libraryItemID := uuid.New().String()

		dbModel := &PodcastDbModel{
			ID:                   podcastID,
			Title:                title,
			Author:               author,
			FeedURL:              feedURL,
			Description:          description,
			Language:             language,
			Explicit:             explicit,
			AutoDownloadEpisodes: req.AutoDownloadEpisodes,
			Genres:               genres,
		}

		var eps []*podcast.PodcastEpisode
		if feed != nil {
			eps = feed.Episodes
		}

		err = dbInsertPodcast(r.Context(), db, dbModel, libraryItemID, req.FolderID, req.LibraryID, podcastPath, eps)
		if err != nil {
			log.Errorf("[CreatePodcast] dbInsertPodcast failed: %v", err)
			http.Error(w, `{"error": "Failed to insert podcast"}`, http.StatusInternalServerError)
			return
		}

		if req.AutoDownloadEpisodes && feedURL != "" {
			go func() {
				_ = globalPodcastManager.SyncFeed(context.Background(), podcastID)
			}()
		}

		itemMin, err := idb.GetLibraryItemMinifiedByID(db, libraryItemID)
		if err == nil && itemMin != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(itemMin)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"id": "%s"}`, libraryItemID)))
	}
}
