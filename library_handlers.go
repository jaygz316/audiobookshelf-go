package main

import (
	"archive/zip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func serveStaticOrSPA(distPath, routerBasePath string) http.HandlerFunc {
	fileServer := http.FileServer(http.Dir(distPath))
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, routerBasePath) {
			path = strings.TrimPrefix(path, routerBasePath)
		}
		if path == "" {
			path = "/"
		}

		// Check if file exists in distPath
		fullPath := filepath.Join(distPath, filepath.Clean(path))
		stat, err := os.Stat(fullPath)
		if err == nil && !stat.IsDir() {
			// Strip prefix and serve file
			http.StripPrefix(routerBasePath, fileServer).ServeHTTP(w, r)
			return
		}

		// Serve index.html as fallback for Client-side SPA routing
		log.Printf("[SPA] Fallback for GET %s -> index.html", r.URL.Path)
		http.ServeFile(w, r, filepath.Join(distPath, "index.html"))
	}
}

func getCoverFromCache(metadataPath, itemID, width, height, format string) (string, error) {
	cacheFilename := itemID + "_" + width
	if height != "" {
		cacheFilename += "x" + height
	}
	cacheFilename += "." + format
	cachePath := filepath.Join(metadataPath, "cache", "covers", cacheFilename)
	if _, err := os.Stat(cachePath); err != nil {
		return "", err
	}
	return cachePath, nil
}

func serveCover(db *sql.DB, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		var itemID string
		for i, part := range parts {
			if part == "items" && i+1 < len(parts) {
				itemID = parts[i+1]
				break
			}
		}

		if itemID == "" {
			http.Error(w, `{"error": "Invalid Item ID"}`, http.StatusBadRequest)
			return
		}

		raw := r.URL.Query().Get("raw") == "1"

		if raw {
			coverPath, err := GetCoverPath(db, itemID)
			if err != nil || coverPath == "" {
				http.NotFound(w, r)
				return
			}
			if r.URL.Query().Get("ts") != "" {
				w.Header().Set("Cache-Control", "private, max-age=86400")
			}
			http.ServeFile(w, r, coverPath)
			return
		}

		// Non-raw: check cover cache first
		format := r.URL.Query().Get("format")
		if format == "" {
			if strings.Contains(r.Header.Get("Accept"), "image/webp") {
				format = "webp"
			} else {
				format = "jpeg"
			}
		}
		width := r.URL.Query().Get("width")
		if width == "" {
			width = "400"
		}
		height := r.URL.Query().Get("height")

		cachePath, err := getCoverFromCache(metadataPath, itemID, width, height, format)
		if err == nil {
			if r.URL.Query().Get("ts") != "" {
				w.Header().Set("Cache-Control", "private, max-age=86400")
			}
			w.Header().Set("Content-Type", "image/"+format)
			http.ServeFile(w, r, cachePath)
			return
		}

		// Cache miss fallback: serve the raw cover natively
		log.Printf("[Cover] Cache miss. Serving raw cover.")
		coverPath, err := GetCoverPath(db, itemID)
		if err != nil || coverPath == "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, coverPath)
	}
}

func streamDirAsZip(w io.Writer, dirPath string) error {
	zw := zip.NewWriter(w)
	defer zw.Close()

	return filepath.Walk(dirPath, func(path string, fileInfo os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if fileInfo.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		header, err := zip.FileInfoHeader(fileInfo)
		if err != nil {
			return err
		}
		header.Name = rel
		header.Method = zip.Deflate

		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		_, err = io.Copy(writer, f)
		return err
	})
}

func serveDownload(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*UserSession)

		if !user.CanDownload {
			log.Printf("[Download] Forbidden: User %s does not have download permissions", user.Username)
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		parts := strings.Split(r.URL.Path, "/")
		var itemID string
		for i, part := range parts {
			if part == "items" && i+1 < len(parts) {
				itemID = parts[i+1]
				break
			}
		}

		if itemID == "" {
			http.Error(w, `{"error": "Invalid Item ID"}`, http.StatusBadRequest)
			return
		}

		info, err := GetLibraryItemDownloadInfo(db, itemID)
		if err != nil {
			log.Printf("[Download] Failed to get library item info: %v", err)
			http.Error(w, `{"error": "Library item not found"}`, http.StatusNotFound)
			return
		}

		log.Printf("[Download] User %s requested download for item %s (isFile: %t)", user.Username, itemID, info.IsFile)

		if info.IsFile {
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(info.RelPath)))
			http.ServeFile(w, r, info.Path)
			return
		}

		// Directory zip downloads: zip on-the-fly in Go
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q.zip", filepath.Base(info.Path)))
		if err := streamDirAsZip(w, info.Path); err != nil {
			log.Printf("[Download] Directory zip failed: %v", err)
		}
	}
}

func handleGetLibraries(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*UserSession)

		libs, err := GetLibraries(db)
		if err != nil {
			log.Printf("[Go] Failed to get libraries: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		var filteredLibs []*LibraryJSON = []*LibraryJSON{}
		includeStats := strings.Contains(r.URL.Query().Get("include"), "stats")

		for _, lib := range libs {
			if user.CanAccessLibrary(lib.ID) {
				if includeStats {
					var stats *LibraryStats
					var err error
					if lib.MediaType == "book" {
						stats, err = GetBookLibraryStats(db, lib.ID)
					} else if lib.MediaType == "podcast" {
						stats, err = GetPodcastLibraryStats(db, lib.ID)
					}
					if err == nil {
						lib.Stats = stats
					}
				}
				filteredLibs = append(filteredLibs, lib)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"libraries": filteredLibs,
		})
	}
}

func handleGetLibraryByID(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*UserSession)

		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		if strings.Contains(r.URL.RawQuery, "include=filterdata") {
			fd, err := getLibraryFilterDataGo(db, libraryID)
			if err != nil {
				log.Printf("[Library getFilterData] Error: %v", err)
				http.Error(w, `{"error": "Failed to load filter data"}`, http.StatusInternalServerError)
				return
			}
			lib, err := GetLibraryByID(db, libraryID)
			if err != nil || lib == nil {
				http.Error(w, `{"error": "Library not found"}`, http.StatusNotFound)
				return
			}
			playlists, err := queryPlaylistsForUserAndLibrary(r.Context(), db, user.ID, libraryID)
			numPlaylists := 0
			if err == nil {
				numPlaylists = len(playlists)
			}
			responsePayload := map[string]interface{}{
				"library":          lib,
				"filterdata":       fd,
				"issues":           fd.NumIssues,
				"numUserPlaylists": numPlaylists,
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(responsePayload)
			return
		}

		lib, err := GetLibraryByID(db, libraryID)
		if err != nil {
			log.Printf("[Go] Failed to get library %s: %v", libraryID, err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		if lib == nil {
			http.Error(w, "Library not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lib)
	}
}

func handleGetLibraryItems(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*UserSession)

		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		lib, err := GetLibraryByID(db, libraryID)
		if err != nil {
			log.Printf("[Go] Failed to get library %s: %v", libraryID, err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		if lib == nil {
			http.Error(w, "Library not found", http.StatusNotFound)
			return
		}

		q := r.URL.Query()
		limitVal := 0
		if q.Get("limit") != "" {
			fmt.Sscanf(q.Get("limit"), "%d", &limitVal)
		}
		pageVal := 0
		if q.Get("page") != "" {
			fmt.Sscanf(q.Get("page"), "%d", &pageVal)
		}

		sortBy := q.Get("sort")
		sortDesc := q.Get("desc") == "1"
		filterBy := q.Get("filter")
		minified := q.Get("minified") == "1"
		collapseseries := q.Get("collapseseries") == "1"
		include := q.Get("include")

		var includeArray []string
		if include != "" {
			for _, part := range strings.Split(include, ",") {
				includeArray = append(includeArray, strings.TrimSpace(part))
			}
		}

		opts := GetFilteredLibraryItemsOptions{
			LibraryID:      libraryID,
			User:           user,
			FilterBy:       filterBy,
			SortBy:         sortBy,
			SortDesc:       sortDesc,
			Limit:          limitVal,
			Page:           pageVal,
			CollapseSeries: collapseseries,
			Include:        includeArray,
			MediaType:      lib.MediaType,
			Minified:       minified,
		}

		results, total, err := GetFilteredLibraryItems(db, opts)
		if err != nil {
			log.Printf("[Go] Failed to get filtered items for library %s: %v", libraryID, err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		payload := map[string]interface{}{
			"results":        results,
			"total":          total,
			"limit":          limitVal,
			"page":           pageVal,
			"sortBy":         sortBy,
			"sortDesc":       sortDesc,
			"filterBy":       filterBy,
			"mediaType":      lib.MediaType,
			"minified":       minified,
			"collapseseries": collapseseries,
			"include":        include,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

type Shelf struct {
	ID             string                     `json:"id"`
	Label          string                     `json:"label"`
	LabelStringKey string                     `json:"labelStringKey"`
	Type           string                     `json:"type"`
	Entities       []*LibraryItemMinifiedJSON `json:"entities"`
}

func fetchProgressShelves(db *sql.DB, libraryID string, user *UserSession, limitVal int, mediaType string) ([]Shelf, error) {
	var shelves []Shelf
	optsProgress := GetFilteredLibraryItemsOptions{
		LibraryID:      libraryID,
		User:           user,
		FilterBy:       "progress.in-progress",
		SortBy:         "progress",
		SortDesc:       true,
		Limit:          limitVal,
		Page:           0,
		MediaType:      mediaType,
		Minified:       true,
	}
	progressItems, _, err := GetFilteredLibraryItems(db, optsProgress)
	if err != nil {
		return nil, err
	}

	if len(progressItems) > 0 {
		if mediaType == "book" {
			var listeningItems []*LibraryItemMinifiedJSON
			var readingItems []*LibraryItemMinifiedJSON

			for _, item := range progressItems {
				if item.IsMissing || item.IsInvalid {
					continue
				}
				bookMin, ok := item.Media.(*BookMinifiedJSON)
				if ok && bookMin.NumAudioFiles > 0 {
					listeningItems = append(listeningItems, item)
				} else {
					readingItems = append(readingItems, item)
				}
			}

			if len(listeningItems) > 0 {
				shelves = append(shelves, Shelf{
					ID:             "continue-listening",
					Label:          "Continue Listening",
					LabelStringKey: "LabelContinueListening",
					Type:           "book",
					Entities:       listeningItems,
				})
			}
			if len(readingItems) > 0 {
				shelves = append(shelves, Shelf{
					ID:             "continue-reading",
					Label:          "Continue Reading",
					LabelStringKey: "LabelContinueReading",
					Type:           "book",
					Entities:       readingItems,
				})
			}
		} else if mediaType == "podcast" {
			var filteredProgress []*LibraryItemMinifiedJSON
			for _, item := range progressItems {
				if item.IsMissing || item.IsInvalid {
					continue
				}
				filteredProgress = append(filteredProgress, item)
			}
			if len(filteredProgress) > 0 {
				shelves = append(shelves, Shelf{
					ID:             "continue-listening",
					Label:          "Continue Listening",
					LabelStringKey: "LabelContinueListening",
					Type:           "episode",
					Entities:       filteredProgress,
				})
			}
		}
	}
	return shelves, nil
}

func fetchRecentlyAddedShelf(db *sql.DB, libraryID string, user *UserSession, limitVal int, mediaType string) (*Shelf, error) {
	optsRecent := GetFilteredLibraryItemsOptions{
		LibraryID:      libraryID,
		User:           user,
		SortBy:         "addedAt",
		SortDesc:       true,
		Limit:          limitVal,
		Page:           0,
		MediaType:      mediaType,
		Minified:       true,
	}
	recentItems, _, err := GetFilteredLibraryItems(db, optsRecent)
	if err != nil {
		return nil, err
	}

	if len(recentItems) > 0 {
		var filteredRecent []*LibraryItemMinifiedJSON
		for _, item := range recentItems {
			if item.IsMissing || item.IsInvalid {
				continue
			}
			filteredRecent = append(filteredRecent, item)
		}
		if len(filteredRecent) > 0 {
			return &Shelf{
				ID:             "recently-added",
				Label:          "Recently Added",
				LabelStringKey: "LabelRecentlyAdded",
				Type:           mediaType,
				Entities:       filteredRecent,
			}, nil
		}
	}
	return nil, nil
}

func handleGetLibraryPersonalized(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*UserSession)

		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		lib, err := GetLibraryByID(db, libraryID)
		if err != nil {
			log.Printf("[Go] Failed to get library %s: %v", libraryID, err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		if lib == nil {
			http.Error(w, "Library not found", http.StatusNotFound)
			return
		}

		q := r.URL.Query()
		limitVal := 20
		if q.Get("limit") != "" {
			fmt.Sscanf(q.Get("limit"), "%d", &limitVal)
		}

		var shelves []Shelf

		// 1. Fetch in-progress items
		progressShelves, err := fetchProgressShelves(db, libraryID, user, limitVal, lib.MediaType)
		if err == nil && len(progressShelves) > 0 {
			shelves = append(shelves, progressShelves...)
		}

		// 2. Fetch recently added items
		recentShelf, err := fetchRecentlyAddedShelf(db, libraryID, user, limitVal, lib.MediaType)
		if err == nil && recentShelf != nil {
			shelves = append(shelves, *recentShelf)
		}

		w.Header().Set("Content-Type", "application/json")
		if shelves == nil {
			shelves = []Shelf{}
		}
		json.NewEncoder(w).Encode(shelves)
	}
}

func handleCreateLibrary(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*UserSession)

		if user.Type != "admin" && user.Type != "root" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var payload CreateLibraryPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		if payload.Name == "" {
			http.Error(w, `{"error": "Name is required"}`, http.StatusBadRequest)
			return
		}

		for i, f := range payload.Folders {
			fpath := f.FullPath
			if fpath == "" {
				fpath = f.Path
			}
			if fpath == "" {
				http.Error(w, `{"error": "Folder path is required"}`, http.StatusBadRequest)
				return
			}
			absPath, err := filepath.Abs(fpath)
			if err != nil {
				absPath = fpath
			}
			absPath = filepath.ToSlash(absPath)
			if err := os.MkdirAll(absPath, 0755); err != nil {
				log.Printf("Failed to create folder directory %s: %v", absPath, err)
				http.Error(w, fmt.Sprintf(`{"error": "Invalid folder directory %s"}`, absPath), http.StatusBadRequest)
				return
			}
			payload.Folders[i].Path = absPath
		}

		lib, err := CreateLibrary(db, &payload)
		if err != nil {
			log.Printf("[Go] Failed to create library: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lib)
	}
}

func handleUpdateLibrary(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*UserSession)

		if user.Type != "admin" && user.Type != "root" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var payload UpdateLibraryPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		if payload.Folders != nil {
			for i, f := range payload.Folders {
				if f.ID == "" {
					fpath := f.FullPath
					if fpath == "" {
						fpath = f.Path
					}
					if fpath == "" {
						http.Error(w, `{"error": "Folder path is required"}`, http.StatusBadRequest)
						return
					}
					absPath, err := filepath.Abs(fpath)
					if err != nil {
						absPath = fpath
					}
					absPath = filepath.ToSlash(absPath)
					if err := os.MkdirAll(absPath, 0755); err != nil {
						log.Printf("Failed to create folder directory %s: %v", absPath, err)
						http.Error(w, fmt.Sprintf(`{"error": "Invalid folder directory %s"}`, absPath), http.StatusBadRequest)
						return
					}
					payload.Folders[i].Path = absPath
				}
			}
		}

		lib, err := UpdateLibrary(db, libraryID, &payload)
		if err != nil {
			log.Printf("[Go] Failed to update library %s: %v", libraryID, err)
			if err.Error() == "library not found" {
				http.Error(w, `{"error": "Library not found"}`, http.StatusNotFound)
			} else {
				http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lib)
	}
}

func handleDeleteLibrary(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*UserSession)

		if user.Type != "admin" && user.Type != "root" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		lib, err := DeleteLibrary(db, libraryID)
		if err != nil {
			log.Printf("[Go] Failed to delete library %s: %v", libraryID, err)
			if err.Error() == "library not found" {
				http.Error(w, `{"error": "Library not found"}`, http.StatusNotFound)
			} else {
				http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lib)
	}
}

func handleGetLibraryFilterData(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*UserSession)

		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		fd, err := getLibraryFilterDataGo(db, libraryID)
		if err != nil {
			log.Printf("[Library getFilterData] Error: %v", err)
			http.Error(w, `{"error": "Failed to load filter data"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fd)
	}
}
