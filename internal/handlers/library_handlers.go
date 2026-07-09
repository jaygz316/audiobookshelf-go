package handlers

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/doyensec/safeurl"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	isocket "audiobookshelf/internal/socket"
)

func serveStaticOrSPA(fSys fs.FS, routerBasePath string) http.HandlerFunc {
	if fSys == nil {
		// Fallback to frontend directory FS if subFS is nil
		fSys = os.DirFS("frontend")
	}

	fileServer := http.FileServer(http.FS(fSys))
	return func(w http.ResponseWriter, r *http.Request) {
		allowIframe := false
		if globalDB != nil {
			if settings, err := idb.GetServerSettings(globalDB); err == nil && settings != nil {
				allowIframe = settings.AllowIframe
			}
		}
		if !allowIframe {
			w.Header().Set("X-Frame-Options", "SAMEORIGIN")
			w.Header().Set("Content-Security-Policy", "frame-ancestors 'self'")
		}

		reqPath := r.URL.Path
		if strings.HasPrefix(reqPath, routerBasePath) {
			reqPath = strings.TrimPrefix(reqPath, routerBasePath)
		}
		if reqPath == "" {
			reqPath = "/"
		}

		cleanedPath := path.Clean("/" + reqPath)
		if cleanedPath == "/" {
			cleanedPath = "."
		} else {
			cleanedPath = cleanedPath[1:]
		}

		if cleanedPath == "index.html" {
			data, err := fs.ReadFile(fSys, "index.html")
			if err == nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(data)
				return
			}
		}

		file, err := fSys.Open(cleanedPath)
		var isDir bool
		if err == nil {
			stat, statErr := file.Stat()
			if statErr == nil && stat.IsDir() {
				isDir = true
			}
			file.Close()
		}

		if err == nil && !isDir {
			http.StripPrefix(routerBasePath, fileServer).ServeHTTP(w, r)
			return
		}

		// Serve index.html as fallback for Client-side SPA routing
		log.Printf("[SPA] Fallback for GET %s -> index.html", r.URL.Path)
		data, err := fs.ReadFile(fSys, "index.html")
		if err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>Audiobookshelf Go Gateway</body></html>"))
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

func resizeImage(coverPath, cachePath, width, height, format string) error {
	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Build ffmpeg filter
	filter := fmt.Sprintf("scale=%s:-1", width)
	if height != "" {
		filter = fmt.Sprintf("scale=%s:%s", width, height)
	}

	args := []string{
		"-y",
		"-i", coverPath,
		"-vf", filter,
		cachePath,
	}

	cmd := exec.Command("ffmpeg", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg resize failed: %v, output: %s", err, string(output))
	}
	return nil
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
			coverPath, err := idb.GetCoverPath(db, itemID)
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

		// Validate parameters to prevent command/filter injection
		for _, char := range width {
			if char < '0' || char > '9' {
				http.Error(w, "Invalid width", http.StatusBadRequest)
				return
			}
		}
		for _, char := range height {
			if char < '0' || char > '9' {
				http.Error(w, "Invalid height", http.StatusBadRequest)
				return
			}
		}
		if format != "webp" && format != "jpeg" && format != "jpg" && format != "png" {
			http.Error(w, "Invalid format", http.StatusBadRequest)
			return
		}

		cachePath, err := getCoverFromCache(metadataPath, itemID, width, height, format)
		if err == nil {
			if r.URL.Query().Get("ts") != "" {
				w.Header().Set("Cache-Control", "private, max-age=86400")
			}
			w.Header().Set("Content-Type", "image/"+format)
			http.ServeFile(w, r, cachePath)
			return
		}

		// Cache miss: generate the resized cover
		coverPath, err := idb.GetCoverPath(db, itemID)
		if err == nil && coverPath != "" {
			cacheFilename := itemID + "_" + width
			if height != "" {
				cacheFilename += "x" + height
			}
			cacheFilename += "." + format
			cachePath = filepath.Join(metadataPath, "cache", "covers", cacheFilename)

			errResize := resizeImage(coverPath, cachePath, width, height, format)
			if errResize == nil {
				if r.URL.Query().Get("ts") != "" {
					w.Header().Set("Cache-Control", "private, max-age=86400")
				}
				w.Header().Set("Content-Type", "image/"+format)
				http.ServeFile(w, r, cachePath)
				return
			}
			log.Printf("[Cover] Resize failed for item %s: %v. Falling back to raw cover.", itemID, errResize)
		}

		// Cache miss fallback: serve the raw cover natively
		log.Printf("[Cover] Cache miss. Serving raw cover.")
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
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		if !user.CanDownload {
			log.Printf("[Download] Forbidden: idb.User %s does not have download permissions", user.Username)
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

		info, err := idb.GetLibraryItemDownloadInfo(db, itemID)
		if err != nil {
			log.Printf("[Download] Failed to get library item info: %v", err)
			http.Error(w, `{"error": "Library item not found"}`, http.StatusNotFound)
			return
		}

		log.Printf("[Download] idb.User %s requested download for item %s (isFile: %t)", user.Username, itemID, info.IsFile)

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

func HandleGetLibraries(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		libs, err := idb.GetLibraries(db)
		if err != nil {
			log.Printf("[Go] Failed to get libraries: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		var filteredLibs []*idb.LibraryJSON = []*idb.LibraryJSON{}
		includeStats := strings.Contains(r.URL.Query().Get("include"), "stats")

		for _, lib := range libs {
			if user.CanAccessLibrary(lib.ID) {
				if includeStats {
					var stats *idb.LibraryStats
					var err error
					if lib.MediaType == "book" {
						stats, err = idb.GetBookLibraryStats(db, lib.ID)
					} else if lib.MediaType == "podcast" {
						stats, err = idb.GetPodcastLibraryStats(db, lib.ID)
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

func HandleGetLibraryByID(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		if strings.Contains(r.URL.RawQuery, "include=filterdata") {
			fd, err := idb.GetLibraryFilterDataGo(db, libraryID)
			if err != nil {
				log.Printf("[Library getFilterData] Error: %v", err)
				http.Error(w, `{"error": "Failed to load filter data"}`, http.StatusInternalServerError)
				return
			}
			lib, err := idb.GetLibraryByID(db, libraryID)
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

		lib, err := idb.GetLibraryByID(db, libraryID)
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

func HandleGetLibraryItems(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		lib, err := idb.GetLibraryByID(db, libraryID)
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

		opts := idb.GetFilteredLibraryItemsOptions{
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

		results, total, err := idb.GetFilteredLibraryItems(db, opts)
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
	ID             string                         `json:"id"`
	Label          string                         `json:"label"`
	LabelStringKey string                         `json:"labelStringKey"`
	Type           string                         `json:"type"`
	Entities       []*idb.LibraryItemMinifiedJSON `json:"entities"`
}

func fetchProgressShelves(db *sql.DB, libraryID string, user *core.UserSession, limitVal int, mediaType string) ([]Shelf, error) {
	var shelves []Shelf
	optsProgress := idb.GetFilteredLibraryItemsOptions{
		LibraryID: libraryID,
		User:      user,
		FilterBy:  "progress.in-progress",
		SortBy:    "progress",
		SortDesc:  true,
		Limit:     limitVal,
		Page:      0,
		MediaType: mediaType,
		Minified:  true,
	}
	progressItems, _, err := idb.GetFilteredLibraryItems(db, optsProgress)
	if err != nil {
		return nil, err
	}

	if len(progressItems) > 0 {
		if mediaType == "book" {
			var listeningItems []*idb.LibraryItemMinifiedJSON
			var readingItems []*idb.LibraryItemMinifiedJSON

			for _, item := range progressItems {
				if item.IsMissing || item.IsInvalid {
					continue
				}
				bookMin, ok := item.Media.(*idb.BookMinifiedJSON)
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
			var filteredProgress []*idb.LibraryItemMinifiedJSON
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

func fetchRecentlyAddedShelf(db *sql.DB, libraryID string, user *core.UserSession, limitVal int, mediaType string) (*Shelf, error) {
	optsRecent := idb.GetFilteredLibraryItemsOptions{
		LibraryID: libraryID,
		User:      user,
		SortBy:    "addedAt",
		SortDesc:  true,
		Limit:     limitVal,
		Page:      0,
		MediaType: mediaType,
		Minified:  true,
	}
	recentItems, _, err := idb.GetFilteredLibraryItems(db, optsRecent)
	if err != nil {
		return nil, err
	}

	if len(recentItems) > 0 {
		var filteredRecent []*idb.LibraryItemMinifiedJSON
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

func HandleGetLibraryPersonalized(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		lib, err := idb.GetLibraryByID(db, libraryID)
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

func HandleCreateLibrary(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		if user.Type != "admin" && user.Type != "root" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var payload idb.CreateLibraryPayload
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

		lib, err := idb.CreateLibrary(db, &payload)
		if err != nil {
			log.Printf("[Go] Failed to create library: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lib)
	}
}

func HandleUpdateLibrary(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		if user.Type != "admin" && user.Type != "root" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var payload idb.UpdateLibraryPayload
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

		lib, err := idb.UpdateLibrary(db, libraryID, &payload)
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

func HandleDeleteLibrary(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		if user.Type != "admin" && user.Type != "root" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		lib, err := idb.DeleteLibrary(db, libraryID)
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
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		fd, err := idb.GetLibraryFilterDataGo(db, libraryID)
		if err != nil {
			log.Printf("[Library getFilterData] Error: %v", err)
			http.Error(w, `{"error": "Failed to load filter data"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fd)
	}
}

func handleGetLibraryStats(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		lib, err := idb.GetLibraryByID(db, libraryID)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, `{"error": "Library not found"}`, http.StatusNotFound)
			} else {
				log.Printf("[idb.LibraryStats] Failed to get library %s: %v", libraryID, err)
				http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			}
			return
		}
		if lib == nil {
			http.Error(w, "Library not found", http.StatusNotFound)
			return
		}

		var stats *idb.LibraryStats
		if lib.MediaType == "book" {
			stats, err = idb.GetBookLibraryStats(db, libraryID)
		} else {
			stats, err = idb.GetPodcastLibraryStats(db, libraryID)
		}
		if err != nil {
			log.Printf("[idb.LibraryStats] Failed to get stats for library %s: %v", libraryID, err)
			http.Error(w, `{"error": "Failed to load library stats"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stats)
	}
}

var coverHTTPClient *safeurl.WrappedClient

func init() {
	builder := safeurl.GetConfigBuilder()
	if os.Getenv("BYPASS_SAFEURL") == "true" {
		builder = builder.SetAllowedIPs("127.0.0.1", "::1")
		var ports []int
		for p := 1; p <= 65535; p++ {
			ports = append(ports, p)
		}
		builder = builder.SetAllowedPorts(ports...)
	}
	config := builder.Build()
	coverHTTPClient = safeurl.Client(config)
}

func handleUpdateCoverFromURL(db *sql.DB, cfg *core.Config, itemID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /api/items/%s/cover-from-url", itemID)

		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)
		if user.Type != "root" && user.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var body struct {
			CoverURL string `json:"coverUrl"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		if body.CoverURL == "" {
			http.Error(w, `{"error": "coverUrl is required"}`, http.StatusBadRequest)
			return
		}

		destPath, err := downloadCoverFromURL(r.Context(), db, itemID, body.CoverURL, cfg.MetadataPath)
		if err != nil {
			log.Printf("[Cover From URL] Failed: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		if isocket.GlobalAuth != nil {
			if minItem, err := idb.GetLibraryItemMinifiedByID(db, itemID); err == nil {
				EmitLibraryItemEvent("item_updated", minItem)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":   true,
			"coverPath": destPath,
		})
	}
}

func downloadCoverFromURL(ctx context.Context, db *sql.DB, itemID string, coverURL string, metadataPath string) (string, error) {
	if coverURL == "" {
		return "", fmt.Errorf("empty cover URL")
	}

	// 1. Resolve media type and ID, path, and isFile
	var mediaType, mediaID string
	var itemPath string
	var isFile int
	err := db.QueryRow("SELECT mediaType, mediaId, path, isFile FROM libraryItems WHERE id = ?", itemID).Scan(&mediaType, &mediaID, &itemPath, &isFile)
	if err != nil {
		return "", err
	}

	// 2. Fetch cover image using coverHTTPClient
	req, err := http.NewRequestWithContext(ctx, "GET", coverURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := coverHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch cover from URL, status: %d", resp.StatusCode)
	}

	// Determine extension based on Content-Type
	ext := ".jpg"
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "image/png") {
		ext = ".png"
	} else if strings.Contains(contentType, "image/webp") {
		ext = ".webp"
	} else if strings.Contains(contentType, "image/gif") {
		ext = ".gif"
	}

	// 3. Determine where to save the file
	destPath := ""
	settings, err := idb.GetServerSettings(db)
	if err == nil && settings != nil && settings.MetadataCoverWithItem {
		folder := itemPath
		if isFile != 0 {
			folder = filepath.Dir(itemPath)
		}
		destPath = filepath.Join(folder, "cover"+ext)
	} else {
		var existingCoverPath sql.NullString
		if mediaType == "book" {
			_ = db.QueryRow("SELECT coverPath FROM books WHERE id = ?", mediaID).Scan(&existingCoverPath)
		} else if mediaType == "podcast" {
			_ = db.QueryRow("SELECT coverPath FROM podcasts WHERE id = ?", mediaID).Scan(&existingCoverPath)
		}

		if existingCoverPath.Valid && existingCoverPath.String != "" {
			destPath = existingCoverPath.String
		} else {
			// Save inside metadata/items/{itemID}/cover{ext}
			itemDir := filepath.Join(metadataPath, "items", itemID)
			if err := os.MkdirAll(itemDir, 0755); err != nil {
				return "", err
			}
			destPath = filepath.Join(itemDir, "cover"+ext)
		}
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return "", err
	}

	// 4. Save the file
	out, err := os.Create(destPath)
	if err != nil {
		// If existingCoverPath is not writeable, fallback to metadata items dir
		itemDir := filepath.Join(metadataPath, "items", itemID)
		if err := os.MkdirAll(itemDir, 0755); err == nil {
			destPath = filepath.Join(itemDir, "cover"+ext)
			out, err = os.Create(destPath)
		}
		if err != nil {
			return "", err
		}
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", err
	}

	// Normalize to forward slashes for cross-platform DB consistency
	destPath = filepath.ToSlash(destPath)

	// 5. Update DB
	if mediaType == "book" {
		_, err = db.Exec("UPDATE books SET coverPath = ? WHERE id = ?", destPath, mediaID)
	} else if mediaType == "podcast" {
		_, err = db.Exec("UPDATE podcasts SET coverPath = ? WHERE id = ?", destPath, mediaID)
	}
	if err != nil {
		return "", err
	}

	// Update libraryItems updatedAt to trigger cache bust on UI
	nowStr := time.Now().Format("2006-01-02 15:04:05.000")
	_, _ = db.Exec("UPDATE libraryItems SET updatedAt = ? WHERE id = ?", nowStr, itemID)

	// 6. Clear cached covers for this item to ensure new cover is loaded
	cachePattern := filepath.Join(metadataPath, "cache", "covers", itemID+"_*")
	if files, err := filepath.Glob(cachePattern); err == nil {
		for _, f := range files {
			_ = os.Remove(f)
		}
	}

	return destPath, nil
}
