package handlers

import (
	log "audiobookshelf/internal/logger"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/doyensec/safeurl"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	isocket "audiobookshelf/internal/socket"
	"audiobookshelf/internal/utils"
)

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

		if itemID == "" || strings.Contains(itemID, "..") || strings.Contains(itemID, "\\") {
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
			if !utils.IsSafeFilePath(db, metadataPath, coverPath) {
				log.Warnf("[Cover] Raw cover path traversal blocked: %s", coverPath)
				http.Error(w, "Forbidden", http.StatusForbidden)
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
			if !utils.IsSafeFilePath(db, metadataPath, cachePath) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
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
			if !utils.IsSafeFilePath(db, metadataPath, coverPath) {
				log.Warnf("[Cover] Resized cover source path traversal blocked: %s", coverPath)
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
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
			log.Errorf("[Cover] Resize failed for item %s: %v. Falling back to raw cover.", itemID, errResize)
		}

		// Cache miss fallback: serve the raw cover natively
		log.Infof("[Cover] Cache miss. Serving raw cover.")
		if err != nil || coverPath == "" {
			http.NotFound(w, r)
			return
		}
		if !utils.IsSafeFilePath(db, metadataPath, coverPath) {
			log.Warnf("[Cover] Cache miss fallback cover path traversal blocked: %s", coverPath)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		http.ServeFile(w, r, coverPath)
	}
}

func handleUpdateCoverFromURL(db *sql.DB, cfg *core.Config, itemID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST /api/items/%s/cover-from-url", itemID)

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
			log.Errorf("[Cover From URL] Failed: %v", err)
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

	if !utils.IsSafeFilePath(db, metadataPath, destPath) {
		return "", fmt.Errorf("forbidden: unsafe cover destination path")
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
		destPath = filepath.Join(itemDir, "cover"+ext)
		if !utils.IsSafeFilePath(db, metadataPath, destPath) {
			return "", fmt.Errorf("forbidden: unsafe fallback cover destination path")
		}
		if err := os.MkdirAll(itemDir, 0755); err == nil {
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

func handleUploadCover(db *sql.DB, cfg *core.Config, itemID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST /api/items/%s/cover", itemID)

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
		if user.Type != "root" && user.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		if err := r.ParseMultipartForm(10 * 1024 * 1024); err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Multipart form parse failed: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}
		defer r.MultipartForm.RemoveAll()

		file, header, err := r.FormFile("cover")
		if err != nil {
			file, header, err = r.FormFile("file")
		}
		if err != nil {
			file, header, err = r.FormFile("image")
		}
		if err != nil {
			http.Error(w, `{"error": "No cover file uploaded"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()

		var mediaType, mediaID, itemPath string
		var isFile int
		err = db.QueryRow("SELECT mediaType, mediaId, path, isFile FROM libraryItems WHERE id = ?", itemID).Scan(&mediaType, &mediaID, &itemPath, &isFile)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		ext := filepath.Ext(header.Filename)
		if ext == "" {
			ext = ".jpg"
		}
		extLower := strings.ToLower(ext)
		if extLower != ".jpg" && extLower != ".jpeg" && extLower != ".png" && extLower != ".webp" && extLower != ".gif" {
			ext = ".jpg"
		}

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
				itemDir := filepath.Join(cfg.MetadataPath, "items", itemID)
				if err := os.MkdirAll(itemDir, 0755); err != nil {
					http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
					return
				}
				destPath = filepath.Join(itemDir, "cover"+ext)
			}
		}

		if !utils.IsSafeFilePath(db, cfg.MetadataPath, destPath) {
			http.Error(w, `{"error": "forbidden: unsafe cover destination path"}`, http.StatusForbidden)
			return
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		out, err := os.Create(destPath)
		if err != nil {
			itemDir := filepath.Join(cfg.MetadataPath, "items", itemID)
			destPath = filepath.Join(itemDir, "cover"+ext)
			if !utils.IsSafeFilePath(db, cfg.MetadataPath, destPath) {
				http.Error(w, `{"error": "forbidden: unsafe fallback cover destination path"}`, http.StatusForbidden)
				return
			}
			if err := os.MkdirAll(itemDir, 0755); err == nil {
				out, err = os.Create(destPath)
			}
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
		}
		defer out.Close()

		if _, err := io.Copy(out, file); err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		destPath = filepath.ToSlash(destPath)

		if mediaType == "book" {
			_, err = db.Exec("UPDATE books SET coverPath = ? WHERE id = ?", destPath, mediaID)
		} else if mediaType == "podcast" {
			_, err = db.Exec("UPDATE podcasts SET coverPath = ? WHERE id = ?", destPath, mediaID)
		}
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		nowStr := time.Now().Format("2006-01-02 15:04:05.000")
		_, _ = db.Exec("UPDATE libraryItems SET updatedAt = ? WHERE id = ?", nowStr, itemID)

		cachePattern := filepath.Join(cfg.MetadataPath, "cache", "covers", itemID+"_*")
		if files, err := filepath.Glob(cachePattern); err == nil {
			for _, f := range files {
				_ = os.Remove(f)
			}
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
