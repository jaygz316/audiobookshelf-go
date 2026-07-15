package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"audiobookshelf/internal/core"
	iscanner "audiobookshelf/internal/scanner"
	isocket "audiobookshelf/internal/socket"
	"audiobookshelf/internal/utils"
)

// handleUpload handles bulk uploading of audiobooks and podcasts.
func handleUpload(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST /api/upload")
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" && !userSess.CanUpload {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		// Limit the form parsing. ParseMultipartForm processes up to maxMemory in RAM,
		// and any excess is written to temp files on disk. 128MB is a good memory buffer.
		if err := r.ParseMultipartForm(128 * 1024 * 1024); err != nil {
			log.Errorf("[Upload] Multipart parse failed: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "Multipart form parse failed: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}
		defer r.MultipartForm.RemoveAll()

		libraryID := r.FormValue("library")
		if libraryID == "" {
			libraryID = r.FormValue("libraryId")
		}
		if libraryID == "" {
			p := r.URL.Path
			if idx := strings.Index(p, "/api/libraries/"); idx != -1 {
				sub := p[idx+len("/api/libraries/"):]
				parts := strings.Split(sub, "/")
				if len(parts) >= 1 && parts[0] != "" {
					libraryID = parts[0]
				}
			}
		}
		if libraryID == "" {
			http.Error(w, `{"error": "Missing library ID"}`, http.StatusBadRequest)
			return
		}

		folderID := r.FormValue("folder")
		if folderID == "" {
			folderID = r.FormValue("folderId")
		}

		var folderPath string
		var err error
		if folderID != "" {
			err = db.QueryRowContext(r.Context(), "SELECT path FROM libraryFolders WHERE id = ? AND libraryId = ?", folderID, libraryID).Scan(&folderPath)
		} else {
			err = db.QueryRowContext(r.Context(), "SELECT path FROM libraryFolders WHERE libraryId = ? LIMIT 1", libraryID).Scan(&folderPath)
		}

		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, `{"error": "Library folder not found"}`, http.StatusBadRequest)
			} else {
				log.Errorf("[Upload] Database query error: %v", err)
				http.Error(w, `{"error": "Internal database error"}`, http.StatusInternalServerError)
			}
			return
		}

		if folderPath == "" {
			http.Error(w, `{"error": "Library folder path is empty"}`, http.StatusBadRequest)
			return
		}

		// Ensure directory exists
		if err := os.MkdirAll(folderPath, 0755); err != nil {
			log.Errorf("[Upload] Failed to create destination directory %s: %v", folderPath, err)
			http.Error(w, `{"error": "Failed to create destination directory on server"}`, http.StatusInternalServerError)
			return
		}

		filesUploaded := 0
		for _, fileHeaders := range r.MultipartForm.File {
			for _, header := range fileHeaders {
				file, err := header.Open()
				if err != nil {
					log.Errorf("[Upload] Failed to open uploaded file: %v", err)
					http.Error(w, `{"error": "Failed to open uploaded file"}`, http.StatusInternalServerError)
					return
				}
				defer file.Close()

				// To preserve nested file/folder structure (which Go's mime/multipart p.FileName() automatically strips
				// via filepath.Base), we manually parse the filename from the raw Content-Disposition header.
				rawFilename := header.Filename
				if cd := header.Header.Get("Content-Disposition"); cd != "" {
					if _, params, err := mime.ParseMediaType(cd); err == nil {
						if fn := params["filename"]; fn != "" {
							rawFilename = fn
						}
					}
				}

				log.Debugf("[Upload Debug] rawFilename: %q (header.Filename was %q)", rawFilename, header.Filename)
				relPath := rawFilename
				relPath = strings.ReplaceAll(relPath, "\\", "/")
				relPath = path.Clean(relPath)
				relPath = strings.TrimPrefix(relPath, "/")

				if strings.HasPrefix(relPath, "..") || strings.Contains(relPath, "../") {
					log.Warnf("[Upload] Traversal attempt blocked: %s", header.Filename)
					http.Error(w, `{"error": "Invalid file path (directory traversal blocked)"}`, http.StatusBadRequest)
					return
				}

				destPath := filepath.Join(folderPath, relPath)
				if !utils.IsSameOrSubPath(folderPath, destPath) {
					log.Warnf("[Upload] Traversal check failed: %s is not in %s", destPath, folderPath)
					http.Error(w, `{"error": "Invalid file path (directory traversal blocked)"}`, http.StatusBadRequest)
					return
				}

				if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
					log.Errorf("[Upload] Failed to create directories for %s: %v", destPath, err)
					http.Error(w, `{"error": "Failed to create directories on server"}`, http.StatusInternalServerError)
					return
				}

				destFile, err := os.Create(destPath)
				if err != nil {
					log.Errorf("[Upload] Failed to create file %s: %v", destPath, err)
					http.Error(w, `{"error": "Failed to create destination file"}`, http.StatusInternalServerError)
					return
				}
				defer destFile.Close()

				if _, err := io.Copy(destFile, file); err != nil {
					log.Errorf("[Upload] Failed to write file contents for %s: %v", destPath, err)
					http.Error(w, `{"error": "Failed to write file contents"}`, http.StatusInternalServerError)
					return
				}

				filesUploaded++
			}
		}

		// Trigger library scan in background
		go func() {
			if err := iscanner.ScanLibrary(db, libraryID, isocket.GlobalAuth); err != nil {
				log.Errorf("[Upload] Background library scan failed: %v", err)
			}
		}()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("Successfully uploaded %d file(s) and triggered scan.", filesUploaded),
		})
	}
}
