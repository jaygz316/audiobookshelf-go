package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	isocket "audiobookshelf/internal/socket"
	"audiobookshelf/internal/utils"
)

type updateItemPayload struct {
	Title                    string   `json:"title"`
	Subtitle                 string   `json:"subtitle"`
	Authors                  []string `json:"authors"`
	Narrators                []string `json:"narrators"`
	SeriesName               string   `json:"seriesName"`
	SeriesSequence           string   `json:"seriesSequence"`
	Publisher                string   `json:"publisher"`
	PublishedYear            string   `json:"publishedYear"`
	PublishedDate            string   `json:"publishedDate"`
	Description              string   `json:"description"`
	Isbn                     string   `json:"isbn"`
	Asin                     string   `json:"asin"`
	Language                 string   `json:"language"`
	Explicit                 bool     `json:"explicit"`
	Abridged                 bool     `json:"abridged"`
	Tags                     []string `json:"tags"`
	Genres                   []string `json:"genres"`
	LockedFields             []string `json:"lockedFields"`
	AutoDownloadEpisodes     *bool    `json:"autoDownloadEpisodes"`
	AutoDownloadSchedule     *string  `json:"autoDownloadSchedule"`
	MaxEpisodesToKeep        *int     `json:"maxEpisodesToKeep"`
	MaxNewEpisodesToDownload *int     `json:"maxNewEpisodesToDownload"`
	AutoDeletePlayed         *bool    `json:"autoDeletePlayed"`
	SkipIntroDuration        *int     `json:"skipIntroDuration"`
	SkipOutroDuration        *int     `json:"skipOutroDuration"`
}

// handleUpdateLibraryItemByID resolves PATCH /api/items/{id}
func handleUpdateLibraryItemByID(db *sql.DB, itemID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] PATCH /api/items/%s", itemID)

		if strings.Contains(itemID, "..") || strings.Contains(itemID, "/") || strings.Contains(itemID, "\\") {
			http.Error(w, `{"error": "Invalid item ID"}`, http.StatusBadRequest)
			return
		}

		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		if user.Type != "root" && user.Type != "admin" && !user.CanUpdate {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var mediaID, mediaType, libraryID string
		err := db.QueryRow("SELECT mediaId, mediaType, libraryId FROM libraryItems WHERE id = ?", itemID).Scan(&mediaID, &mediaType, &libraryID)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		var payload updateItemPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		prefixes := idb.GetSortingPrefixes(db)

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		nowStr := time.Now().Format("2006-01-02 15:04:05.000")

		if mediaType == "book" {
			if err := updateBookDetails(tx, itemID, mediaID, libraryID, nowStr, prefixes, &payload); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else if mediaType == "podcast" {
			if err := updatePodcastDetails(r, tx, itemID, mediaID, nowStr, prefixes, &payload); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		err = tx.Commit()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeMetadataFile(db, itemID, &payload)

		if isocket.GlobalAuth != nil {
			if minItem, err := idb.GetLibraryItemMinifiedByID(db, itemID); err == nil {
				EmitLibraryItemEvent("item_updated", minItem)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true}`))
	}
}

func writeMetadataFile(db *sql.DB, itemID string, payload *updateItemPayload) {
	srvSettings, srvErr := idb.GetServerSettings(db)
	var metadataPath string
	if srvErr == nil && srvSettings != nil && srvSettings.MetadataMarkdownWithItem {
		var itemPath string
		var isFile int
		dbErr := db.QueryRow("SELECT path, isFile FROM libraryItems WHERE id = ?", itemID).Scan(&itemPath, &isFile)
		if dbErr == nil && itemPath != "" {
			folder := itemPath
			if isFile != 0 {
				folder = filepath.Dir(itemPath)
			}
			metadataPath = filepath.Join(folder, "metadata.json")
		}
	} else {
		itemDir := filepath.Join(MetadataPath, "items", itemID)
		_ = os.MkdirAll(itemDir, 0755)
		metadataPath = filepath.Join(itemDir, "metadata.json")
	}

	if metadataPath != "" && utils.IsSafeFilePath(db, MetadataPath, metadataPath) {
		metaData := map[string]interface{}{
			"title":         payload.Title,
			"subtitle":      payload.Subtitle,
			"authors":       payload.Authors,
			"narrators":     payload.Narrators,
			"publisher":     payload.Publisher,
			"publishedYear": payload.PublishedYear,
			"publishedDate": payload.PublishedDate,
			"description":   payload.Description,
			"isbn":          payload.Isbn,
			"asin":          payload.Asin,
			"language":      payload.Language,
			"explicit":      payload.Explicit,
			"abridged":      payload.Abridged,
			"tags":          payload.Tags,
			"genres":        payload.Genres,
			"lockedFields":  payload.LockedFields,
		}
		metaJSON, marshalErr := json.MarshalIndent(metaData, "", "  ")
		if marshalErr == nil {
			_ = os.WriteFile(metadataPath, metaJSON, 0644)
		}
	}
}
