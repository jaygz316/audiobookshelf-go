package handlers

import (
	log "audiobookshelf/internal/logger"
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/podcast"
	"audiobookshelf/internal/utils"

	"github.com/google/uuid"
)

// opmlOutline represents a single outline element in an OPML document
type opmlOutline struct {
	Text     string        `xml:"text,attr"`
	Title    string        `xml:"title,attr"`
	Type     string        `xml:"type,attr"`
	XMLURL   string        `xml:"xmlUrl,attr"`
	Outlines []opmlOutline `xml:"outline"`
}

// opmlDocument represents the full structure of an OPML file
type opmlDocument struct {
	XMLName  xml.Name      `xml:"opml"`
	Outlines []opmlOutline `xml:"body>outline"`
}

func findFeeds(outlines []opmlOutline) []map[string]string {
	var feeds []map[string]string
	for _, o := range outlines {
		if o.XMLURL != "" {
			title := o.Title
			if title == "" {
				title = o.Text
			}
			feeds = append(feeds, map[string]string{
				"title":   title,
				"feedUrl": o.XMLURL,
			})
		}
		if len(o.Outlines) > 0 {
			feeds = append(feeds, findFeeds(o.Outlines)...)
		}
	}
	return feeds
}

func sanitizeFilename(name string) string {
	invalid := []string{"/", "\\", "?", "%", "*", ":", "|", "\"", "<", ">", "."}
	res := name
	for _, char := range invalid {
		res = strings.ReplaceAll(res, char, "")
	}
	res = strings.TrimSpace(res)
	if res == "" {
		res = "unnamed"
	}
	return res
}

func getTableColumnsTx(tx *sql.Tx, tableName string) map[string]bool {
	cols := make(map[string]bool)
	rows, err := tx.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return cols
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pkey int
		var dfltVal interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltVal, &pkey); err == nil {
			cols[name] = true
		}
	}
	return cols
}

func getTableColumns(db *sql.DB, tableName string) map[string]bool {
	cols := make(map[string]bool)
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return cols
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pkey int
		var dfltVal interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltVal, &pkey); err == nil {
			cols[name] = true
		}
	}
	return cols
}

func explicitInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

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
		nowStr := time.Now().Format("2006-01-02T15:04:05.000Z")

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, `{"error": "Failed to start transaction"}`, http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		cols := getTableColumnsTx(tx, "podcasts")
		var colNames []string
		var placeholders []string
		var args []interface{}

		addCol := func(name string, val interface{}) {
			if cols[name] {
				colNames = append(colNames, name)
				placeholders = append(placeholders, "?")
				args = append(args, val)
			}
		}

		addCol("id", podcastID)
		addCol("title", title)
		addCol("titleIgnorePrefix", title)
		addCol("author", author)
		addCol("feedURL", feedURL)
		addCol("description", description)
		addCol("language", language)
		addCol("explicit", explicitInt(explicit))
		addCol("autoDownloadEpisodes", boolToInt(req.AutoDownloadEpisodes))
		addCol("createdAt", nowStr)
		addCol("updatedAt", nowStr)
		genresJSON, _ := json.Marshal(genres)
		addCol("genres", genresJSON)
		tagsJSON, _ := json.Marshal([]string{})
		addCol("tags", tagsJSON)

		query := fmt.Sprintf("INSERT INTO podcasts (%s) VALUES (%s)", strings.Join(colNames, ", "), strings.Join(placeholders, ", "))
		_, err = tx.Exec(query, args...)
		if err != nil {
			log.Errorf("[CreatePodcast] Insert podcast failed: %v", err)
			http.Error(w, `{"error": "Failed to insert podcast"}`, http.StatusInternalServerError)
			return
		}

		colsLi := getTableColumnsTx(tx, "libraryItems")
		colNames = nil
		placeholders = nil
		args = nil

		addColLi := func(name string, val interface{}) {
			if colsLi[name] {
				colNames = append(colNames, name)
				placeholders = append(placeholders, "?")
				args = append(args, val)
			}
		}

		addColLi("id", libraryItemID)
		addColLi("libraryId", req.LibraryID)
		addColLi("libraryFolderId", req.FolderID)
		addColLi("path", podcastPath)
		relPath, _ := filepath.Rel(folderPath, podcastPath)
		addColLi("relPath", relPath)
		addColLi("isFile", 0)
		addColLi("createdAt", nowStr)
		addColLi("updatedAt", nowStr)
		addColLi("isMissing", 0)
		addColLi("isInvalid", 0)
		addColLi("mediaType", "podcast")
		addColLi("mediaId", podcastID)
		addColLi("title", title)
		addColLi("titleIgnorePrefix", title)

		queryLi := fmt.Sprintf("INSERT INTO libraryItems (%s) VALUES (%s)", strings.Join(colNames, ", "), strings.Join(placeholders, ", "))
		_, err = tx.Exec(queryLi, args...)
		if err != nil {
			log.Errorf("[CreatePodcast] Insert libraryItem failed: %v", err)
			http.Error(w, `{"error": "Failed to insert library item"}`, http.StatusInternalServerError)
			return
		}

		if feed != nil {
			for _, ep := range feed.Episodes {
				epID := uuid.New().String()
				colsEp := getTableColumnsTx(tx, "podcastEpisodes")
				colNames = nil
				placeholders = nil
				args = nil

				addColEp := func(name string, val interface{}) {
					if colsEp[name] {
						colNames = append(colNames, name)
						placeholders = append(placeholders, "?")
						args = append(args, val)
					}
				}

				addColEp("id", epID)
				addColEp("podcastId", podcastID)
				addColEp("title", ep.Title)
				addColEp("description", ep.Description)
				addColEp("enclosureURL", ep.EnclosureURL)
				addColEp("pubDate", ep.PublishedAt)
				addColEp("publishedAt", ep.PublishedAt)
				addColEp("createdAt", nowStr)
				addColEp("updatedAt", nowStr)

				audioFileJSON, _ := json.Marshal(map[string]interface{}{
					"duration": ep.Duration,
				})
				addColEp("audioFile", audioFileJSON)

				queryEp := fmt.Sprintf("INSERT INTO podcastEpisodes (%s) VALUES (%s)", strings.Join(colNames, ", "), strings.Join(placeholders, ", "))
				_, _ = tx.Exec(queryEp, args...)
			}
		}

		if err := tx.Commit(); err != nil {
			http.Error(w, `{"error": "Failed to commit transaction"}`, http.StatusInternalServerError)
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

func handleGetPodcastFeed(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if globalPodcastManager == nil {
			initManagers(db)
		}
		var req struct {
			RSSFeed string `json:"rssFeed"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.RSSFeed == "" {
			http.Error(w, `{"error": "rssFeed parameter is required"}`, http.StatusBadRequest)
			return
		}

		feed, err := globalPodcastManager.FetchFeed(r.Context(), req.RSSFeed)
		if err != nil {
			log.Errorf("[GetPodcastFeed] FetchFeed failed: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "Failed to fetch podcast feed: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		type FeedResponseEpisode struct {
			Title        string  `json:"title"`
			Description  string  `json:"description"`
			PubDate      string  `json:"pubDate"`
			PublishedAt  string  `json:"publishedAt"`
			Duration     float64 `json:"duration"`
			EnclosureURL string  `json:"enclosureUrl"`
		}

		var episodes []*FeedResponseEpisode
		for _, ep := range feed.Episodes {
			episodes = append(episodes, &FeedResponseEpisode{
				Title:        ep.Title,
				Description:  ep.Description,
				PubDate:      ep.PublishedAt,
				PublishedAt:  ep.PublishedAt,
				Duration:     ep.Duration,
				EnclosureURL: ep.EnclosureURL,
			})
		}

		response := map[string]interface{}{
			"podcast": map[string]interface{}{
				"metadata": map[string]interface{}{
					"title":       feed.Title,
					"author":      feed.Author,
					"description": feed.Description,
					"feedUrl":     req.RSSFeed,
				},
				"episodes": episodes,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

func handleParseOPML(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
			OPMLText string `json:"opmlText"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.OPMLText == "" {
			http.Error(w, `{"error": "opmlText parameter is required"}`, http.StatusBadRequest)
			return
		}

		var doc opmlDocument
		if err := xml.Unmarshal([]byte(req.OPMLText), &doc); err != nil {
			log.Errorf("[ParseOPML] Failed to parse XML: %v", err)
			http.Error(w, `{"error": "Failed to parse OPML XML"}`, http.StatusBadRequest)
			return
		}

		feeds := findFeeds(doc.Outlines)
		if feeds == nil {
			feeds = []map[string]string{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"feeds": feeds,
		})
	}
}

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
				nowStr := time.Now().Format("2006-01-02T15:04:05.000Z")

				tx, err := db.Begin()
				if err != nil {
					cancel()
					continue
				}

				cols := getTableColumnsTx(tx, "podcasts")
				var colNames []string
				var placeholders []string
				var args []interface{}

				addCol := func(name string, val interface{}) {
					if cols[name] {
						colNames = append(colNames, name)
						placeholders = append(placeholders, "?")
						args = append(args, val)
					}
				}

				addCol("id", podcastID)
				addCol("title", title)
				addCol("titleIgnorePrefix", title)
				addCol("author", feed.Author)
				addCol("feedURL", feedURL)
				addCol("description", feed.Description)
				addCol("autoDownloadEpisodes", boolToInt(req.AutoDownloadEpisodes))
				addCol("createdAt", nowStr)
				addCol("updatedAt", nowStr)
				genresJSON, _ := json.Marshal([]string{})
				addCol("genres", genresJSON)
				tagsJSON, _ := json.Marshal([]string{})
				addCol("tags", tagsJSON)

				query := fmt.Sprintf("INSERT INTO podcasts (%s) VALUES (%s)", strings.Join(colNames, ", "), strings.Join(placeholders, ", "))
				_, err = tx.Exec(query, args...)
				if err != nil {
					tx.Rollback()
					cancel()
					continue
				}

				colsLi := getTableColumnsTx(tx, "libraryItems")
				colNames = nil
				placeholders = nil
				args = nil

				addColLi := func(name string, val interface{}) {
					if colsLi[name] {
						colNames = append(colNames, name)
						placeholders = append(placeholders, "?")
						args = append(args, val)
					}
				}

				addColLi("id", libraryItemID)
				addColLi("libraryId", req.LibraryID)
				addColLi("libraryFolderId", req.FolderID)
				addColLi("path", podcastPath)
				relPath, _ := filepath.Rel(folderPath, podcastPath)
				addColLi("relPath", relPath)
				addColLi("isFile", 0)
				addColLi("createdAt", nowStr)
				addColLi("updatedAt", nowStr)
				addColLi("isMissing", 0)
				addColLi("isInvalid", 0)
				addColLi("mediaType", "podcast")
				addColLi("mediaId", podcastID)
				addColLi("title", title)
				addColLi("titleIgnorePrefix", title)

				queryLi := fmt.Sprintf("INSERT INTO libraryItems (%s) VALUES (%s)", strings.Join(colNames, ", "), strings.Join(placeholders, ", "))
				_, err = tx.Exec(queryLi, args...)
				if err != nil {
					tx.Rollback()
					cancel()
					continue
				}

				for _, ep := range feed.Episodes {
					epID := uuid.New().String()
					colsEp := getTableColumnsTx(tx, "podcastEpisodes")
					colNames = nil
					placeholders = nil
					args = nil

					addColEp := func(name string, val interface{}) {
						if colsEp[name] {
							colNames = append(colNames, name)
							placeholders = append(placeholders, "?")
							args = append(args, val)
						}
					}

					addColEp("id", epID)
					addColEp("podcastId", podcastID)
					addColEp("title", ep.Title)
					addColEp("description", ep.Description)
					addColEp("enclosureURL", ep.EnclosureURL)
					addColEp("pubDate", ep.PublishedAt)
					addColEp("publishedAt", ep.PublishedAt)
					addColEp("createdAt", nowStr)
					addColEp("updatedAt", nowStr)

					audioFileJSON, _ := json.Marshal(map[string]interface{}{
						"duration": ep.Duration,
					})
					addColEp("audioFile", audioFileJSON)

					queryEp := fmt.Sprintf("INSERT INTO podcastEpisodes (%s) VALUES (%s)", strings.Join(colNames, ", "), strings.Join(placeholders, ", "))
					_, _ = tx.Exec(queryEp, args...)
				}

				if err := tx.Commit(); err == nil {
					if req.AutoDownloadEpisodes {
						_ = globalPodcastManager.SyncFeed(context.Background(), podcastID)
					}
				}
				cancel()
			}
		}()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}
}

func handleCheckNewEpisodes(db *sql.DB, id string) http.HandlerFunc {
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

		var podcastID, libraryID string
		err := db.QueryRow(`
			SELECT p.id, li.libraryId
			FROM podcasts p
			JOIN libraryItems li ON li.mediaId = p.id AND li.mediaType = 'podcast'
			WHERE p.id = ? OR li.id = ?
		`, id, id).Scan(&podcastID, &libraryID)
		if err != nil {
			http.Error(w, `{"error": "Podcast not found"}`, http.StatusNotFound)
			return
		}

		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		err = globalPodcastManager.SyncFeed(r.Context(), podcastID)
		if err != nil {
			log.Errorf("[CheckNewEpisodes] SyncFeed failed: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "Sync failed: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		episodes, err := fetchPodcastEpisodesList(r.Context(), db, podcastID)
		if err != nil {
			http.Error(w, `{"error": "Failed to fetch episodes"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"episodes": episodes,
		})
	}
}

func fetchPodcastEpisodesList(ctx context.Context, db *sql.DB, podcastID string) ([]map[string]interface{}, error) {
	hasPubDate := hasColumn(ctx, db, "podcastEpisodes", "pubDate")
	hasDesc := hasColumn(ctx, db, "podcastEpisodes", "description")
	hasSeason := hasColumn(ctx, db, "podcastEpisodes", "season")
	hasEp := hasColumn(ctx, db, "podcastEpisodes", "episode")
	hasEpType := hasColumn(ctx, db, "podcastEpisodes", "episodeType")
	hasEnclosureURL := hasColumn(ctx, db, "podcastEpisodes", "enclosureURL")

	epQuery := "SELECT id, title, audioFile"
	if hasPubDate {
		epQuery += ", pubDate"
	}
	if hasDesc {
		epQuery += ", description"
	}
	if hasSeason {
		epQuery += ", season"
	}
	if hasEp {
		epQuery += ", episode"
	}
	if hasEpType {
		epQuery += ", episodeType"
	}
	if hasEnclosureURL {
		epQuery += ", enclosureURL"
	}
	epQuery += " FROM podcastEpisodes WHERE podcastId = ?"

	rows, err := db.QueryContext(ctx, epQuery, podcastID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var episodes []map[string]interface{}
	for rows.Next() {
		var epID, epTitle, audioFileStr string
		var pubDateVal, descVal, seasonVal, epVal, epTypeVal, encURLVal sql.NullString

		dest := []interface{}{&epID, &epTitle, &audioFileStr}
		if hasPubDate {
			dest = append(dest, &pubDateVal)
		}
		if hasDesc {
			dest = append(dest, &descVal)
		}
		if hasSeason {
			dest = append(dest, &seasonVal)
		}
		if hasEp {
			dest = append(dest, &epVal)
		}
		if hasEpType {
			dest = append(dest, &epTypeVal)
		}
		if hasEnclosureURL {
			dest = append(dest, &encURLVal)
		}

		if err := rows.Scan(dest...); err == nil {
			var af map[string]interface{}
			_ = json.Unmarshal([]byte(audioFileStr), &af)

			epMap := map[string]interface{}{
				"id":        epID,
				"title":     epTitle,
				"audioFile": af,
			}
			if hasPubDate && pubDateVal.Valid {
				epMap["pubDate"] = pubDateVal.String
			}
			if hasDesc && descVal.Valid {
				epMap["description"] = descVal.String
			}
			if hasSeason && seasonVal.Valid {
				epMap["season"] = seasonVal.String
			}
			if hasEp && epVal.Valid {
				epMap["episode"] = epVal.String
			}
			if hasEpType && epTypeVal.Valid {
				epMap["episodeType"] = epTypeVal.String
			}
			if hasEnclosureURL && encURLVal.Valid {
				epMap["enclosureUrl"] = encURLVal.String
			}

			if af != nil {
				if dur, ok := af["duration"]; ok {
					epMap["duration"] = dur
				}
				if meta, ok := af["metadata"].(map[string]interface{}); ok {
					if sz, ok := meta["size"]; ok {
						epMap["size"] = sz
					}
				}
			}
			episodes = append(episodes, epMap)
		}
	}
	return episodes, nil
}

func handleClearEpisodeQueue(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}
}

func handleGetEpisodeDownloads(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		var podcastID, libraryID string
		err := db.QueryRow(`
			SELECT p.id, li.libraryId
			FROM podcasts p
			JOIN libraryItems li ON li.mediaId = p.id AND li.mediaType = 'podcast'
			WHERE p.id = ? OR li.id = ?
		`, id, id).Scan(&podcastID, &libraryID)
		if err != nil {
			http.Error(w, `{"error": "Podcast not found"}`, http.StatusNotFound)
			return
		}

		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		episodes, err := fetchPodcastEpisodesList(r.Context(), db, podcastID)
		if err != nil {
			http.Error(w, `{"error": "Failed to fetch downloads"}`, http.StatusInternalServerError)
			return
		}

		var downloads []map[string]interface{}
		for _, ep := range episodes {
			if af, ok := ep["audioFile"].(map[string]interface{}); ok && af != nil && len(af) > 0 {
				if meta, ok := af["metadata"].(map[string]interface{}); ok && meta != nil {
					if path, ok := meta["path"].(string); ok && path != "" {
						downloads = append(downloads, ep)
					}
				}
			}
		}

		if downloads == nil {
			downloads = []map[string]interface{}{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"downloads": downloads,
		})
	}
}

func handleSearchEpisode(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		var podcastID, libraryID string
		err := db.QueryRow(`
			SELECT p.id, li.libraryId
			FROM podcasts p
			JOIN libraryItems li ON li.mediaId = p.id AND li.mediaType = 'podcast'
			WHERE p.id = ? OR li.id = ?
		`, id, id).Scan(&podcastID, &libraryID)
		if err != nil {
			http.Error(w, `{"error": "Podcast not found"}`, http.StatusNotFound)
			return
		}

		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		titleQuery := r.URL.Query().Get("title")
		if titleQuery == "" {
			http.Error(w, `{"error": "title parameter is required"}`, http.StatusBadRequest)
			return
		}

		episodes, err := fetchPodcastEpisodesList(r.Context(), db, podcastID)
		if err != nil {
			http.Error(w, `{"error": "Failed to fetch episodes"}`, http.StatusInternalServerError)
			return
		}

		var filtered []map[string]interface{}
		for _, ep := range episodes {
			if title, ok := ep["title"].(string); ok && strings.Contains(strings.ToLower(title), strings.ToLower(titleQuery)) {
				filtered = append(filtered, ep)
			}
		}

		if filtered == nil {
			filtered = []map[string]interface{}{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"episodes": filtered,
		})
	}
}

func handleDownloadEpisodes(db *sql.DB, id string) http.HandlerFunc {
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

		var episodeIDs []string
		if err := json.NewDecoder(r.Body).Decode(&episodeIDs); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		var podcastID, libraryItemID, podcastPath, podcastTitle string
		err := db.QueryRow(`
			SELECT p.id, p.title, li.id, li.path
			FROM podcasts p
			JOIN libraryItems li ON li.mediaId = p.id AND li.mediaType = 'podcast'
			WHERE p.id = ? OR li.id = ?
		`, id, id).Scan(&podcastID, &podcastTitle, &libraryItemID, &podcastPath)
		if err != nil {
			http.Error(w, `{"error": "Podcast not found"}`, http.StatusNotFound)
			return
		}

		for _, epID := range episodeIDs {
			var epTitle, enclosureURL string
			err := db.QueryRow(`
				SELECT title, enclosureURL
				FROM podcastEpisodes
				WHERE id = ? AND podcastId = ?
			`, epID, podcastID).Scan(&epTitle, &enclosureURL)
			if err != nil || enclosureURL == "" {
				continue
			}

			destFile := filepath.Join(podcastPath, sanitizeFilename(epTitle)+".mp3")
			if !utils.IsSafeFilePath(db, MetadataPath, destFile) {
				log.Errorf("[DownloadEpisode] Traversal/Unauthorized path attempt blocked: %s", destFile)
				continue
			}

			if podcast.GlobalQueueManager == nil {
				podcast.InitQueueManager(db, globalPodcastManager)
			}

			podcast.GlobalQueueManager.Enqueue(&podcast.DownloadTask{
				ID:           epID,
				PodcastID:    podcastID,
				PodcastTitle: podcastTitle,
				EpisodeTitle: epTitle,
				EnclosureURL: enclosureURL,
				DestPath:     destFile,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}
}

func handleMatchEpisodes(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"numEpisodesUpdated":0}`))
	}
}

func handleGetEpisode(db *sql.DB, id, episodeId string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		var podcastID, libraryID string
		err := db.QueryRow(`
			SELECT p.id, li.libraryId
			FROM podcasts p
			JOIN libraryItems li ON li.mediaId = p.id AND li.mediaType = 'podcast'
			WHERE p.id = ? OR li.id = ?
		`, id, id).Scan(&podcastID, &libraryID)
		if err != nil {
			http.Error(w, `{"error": "Podcast not found"}`, http.StatusNotFound)
			return
		}

		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		episodes, err := fetchPodcastEpisodesList(r.Context(), db, podcastID)
		if err != nil {
			http.Error(w, `{"error": "Failed to fetch episodes"}`, http.StatusInternalServerError)
			return
		}

		for _, ep := range episodes {
			if ep["id"].(string) == episodeId {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(ep)
				return
			}
		}

		http.Error(w, `{"error": "Episode not found"}`, http.StatusNotFound)
	}
}

func handleUpdateEpisode(db *sql.DB, id, episodeId string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		var podcastID, libraryItemID string
		err := db.QueryRow(`
			SELECT p.id, li.id
			FROM podcasts p
			JOIN libraryItems li ON li.mediaId = p.id AND li.mediaType = 'podcast'
			WHERE p.id = ? OR li.id = ?
		`, id, id).Scan(&podcastID, &libraryItemID)
		if err != nil {
			http.Error(w, `{"error": "Podcast not found"}`, http.StatusNotFound)
			return
		}

		cols := getTableColumns(db, "podcastEpisodes")
		var setParts []string
		var args []interface{}

		for k, v := range req {
			if cols[k] && k != "id" && k != "podcastId" {
				setParts = append(setParts, fmt.Sprintf("%s = ?", k))
				args = append(args, v)
			}
		}

		if len(setParts) > 0 {
			args = append(args, episodeId, podcastID)
			query := fmt.Sprintf("UPDATE podcastEpisodes SET %s WHERE id = ? AND podcastId = ?", strings.Join(setParts, ", "))
			_, err = db.Exec(query, args...)
			if err != nil {
				log.Errorf("[UpdateEpisode] Update failed: %v", err)
				http.Error(w, `{"error": "Update failed"}`, http.StatusInternalServerError)
				return
			}
		}

		itemMin, err := idb.GetLibraryItemMinifiedByID(db, libraryItemID)
		if err == nil && itemMin != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(itemMin)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}
}

func handleDeleteEpisode(db *sql.DB, id, episodeId string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		hardDelete := r.URL.Query().Get("hard") == "1"

		var podcastID, libraryItemID string
		err := db.QueryRow(`
			SELECT p.id, li.id
			FROM podcasts p
			JOIN libraryItems li ON li.mediaId = p.id AND li.mediaType = 'podcast'
			WHERE p.id = ? OR li.id = ?
		`, id, id).Scan(&podcastID, &libraryItemID)
		if err != nil {
			http.Error(w, `{"error": "Podcast not found"}`, http.StatusNotFound)
			return
		}

		var audioFileStr string
		err = db.QueryRow("SELECT audioFile FROM podcastEpisodes WHERE id = ? AND podcastId = ?", episodeId, podcastID).Scan(&audioFileStr)
		if err == nil && audioFileStr != "" {
			var af map[string]interface{}
			if json.Unmarshal([]byte(audioFileStr), &af) == nil && af != nil {
				if meta, ok := af["metadata"].(map[string]interface{}); ok && meta != nil {
					if path, ok := meta["path"].(string); ok && path != "" {
						if utils.IsSafeFilePath(db, MetadataPath, path) {
							if err := os.Remove(path); err != nil {
								log.Errorf("[DeleteEpisode] Failed to remove file %s: %v", path, err)
							}
						} else {
							log.Warnf("[DeleteEpisode] Deletion of unsafe path blocked: %s", path)
						}
					}
				}
			}
		}

		if hardDelete {
			_, err = db.Exec("DELETE FROM podcastEpisodes WHERE id = ? AND podcastId = ?", episodeId, podcastID)
		} else {
			_, err = db.Exec("UPDATE podcastEpisodes SET audioFile = '{}' WHERE id = ? AND podcastId = ?", episodeId, podcastID)
		}

		if err != nil {
			log.Errorf("[DeleteEpisode] Delete failed: %v", err)
			http.Error(w, `{"error": "Delete failed"}`, http.StatusInternalServerError)
			return
		}

		itemMin, err := idb.GetLibraryItemMinifiedByID(db, libraryItemID)
		if err == nil && itemMin != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(itemMin)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}
}

func handlePodcastsDispatch(db *sql.DB, cfg *core.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)
		subPath := strings.TrimPrefix(r.URL.Path, joinPath(cfg.RouterBasePath, "/api/podcasts/"))
		parts := strings.Split(subPath, "/")

		if len(parts) >= 1 && parts[0] == "feed" {
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, handleGetPodcastFeed(db)).ServeHTTP(w, r)
				return
			}
		}
		if len(parts) >= 2 && parts[0] == "opml" && parts[1] == "parse" {
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, handleParseOPML(db)).ServeHTTP(w, r)
				return
			}
		}
		if len(parts) >= 2 && parts[0] == "opml" && parts[1] == "create" {
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, handleBulkCreatePodcasts(db)).ServeHTTP(w, r)
				return
			}
		}

		if len(parts) >= 2 {
			id := parts[0]
			action := parts[1]

			switch action {
			case "checknew":
				if r.Method == http.MethodGet {
					AuthMiddlewareWrapper(db, handleCheckNewEpisodes(db, id)).ServeHTTP(w, r)
					return
				}
			case "clear-queue":
				if r.Method == http.MethodGet {
					AuthMiddlewareWrapper(db, handleClearEpisodeQueue(db, id)).ServeHTTP(w, r)
					return
				}
			case "downloads":
				if r.Method == http.MethodGet {
					AuthMiddlewareWrapper(db, handleGetEpisodeDownloads(db, id)).ServeHTTP(w, r)
					return
				}
			case "search-episode":
				if r.Method == http.MethodGet {
					AuthMiddlewareWrapper(db, handleSearchEpisode(db, id)).ServeHTTP(w, r)
					return
				}
			case "download-episodes":
				if r.Method == http.MethodPost {
					AuthMiddlewareWrapper(db, handleDownloadEpisodes(db, id)).ServeHTTP(w, r)
					return
				}
			case "match-episodes":
				if r.Method == http.MethodPost {
					AuthMiddlewareWrapper(db, handleMatchEpisodes(db, id)).ServeHTTP(w, r)
					return
				}
			case "episode":
				if len(parts) == 3 {
					episodeId := parts[2]
					if r.Method == http.MethodGet {
						AuthMiddlewareWrapper(db, handleGetEpisode(db, id, episodeId)).ServeHTTP(w, r)
						return
					} else if r.Method == http.MethodPatch {
						AuthMiddlewareWrapper(db, handleUpdateEpisode(db, id, episodeId)).ServeHTTP(w, r)
						return
					} else if r.Method == http.MethodDelete {
						AuthMiddlewareWrapper(db, handleDeleteEpisode(db, id, episodeId)).ServeHTTP(w, r)
						return
					}
				}
			}
		}

		http.NotFound(w, r)
	}
}
