package backup

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"audiobookshelf/internal/core"
	log "audiobookshelf/internal/logger"
)

// HandleCreateBackup returns an HTTP handler for POST /api/backups.
func HandleCreateBackup(db *sql.DB, configPath string, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /api/backups")
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		backupDir := GetBackupDirPath(r.Context(), db, metadataPath)
		backups, err := CreateBackup(r.Context(), db, configPath, metadataPath, backupDir)
		if err != nil {
			log.Printf("[Backups] Create failed: %v", err)
			http.Error(w, `{"error": "Failed to create backup"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"backups": backups,
		})
	}
}

// HandleUploadBackup returns an HTTP handler for POST /api/backups/upload.
func HandleUploadBackup(db *sql.DB, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /api/backups/upload")
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		if err := r.ParseMultipartForm(50 * 1024 * 1024); err != nil {
			http.Error(w, `{"error": "Multipart form parse failed"}`, http.StatusBadRequest)
			return
		}
		// Clean up multipart temp files on disk
		defer r.MultipartForm.RemoveAll()

		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, `{"error": "Failed to get file from request"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Path Traversal in Upload File: sanitize using filepath.Base
		safeFilename := filepath.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
		if !strings.HasSuffix(safeFilename, ".audiobookshelf") {
			http.Error(w, `{"error": "Invalid backup file type"}`, http.StatusBadRequest)
			return
		}

		backupDir := GetBackupDirPath(r.Context(), db, metadataPath)
		if err := os.MkdirAll(backupDir, 0755); err != nil {
			http.Error(w, `{"error": "Failed to create backups directory"}`, http.StatusInternalServerError)
			return
		}

		destPath := filepath.Join(backupDir, safeFilename)
		dest, err := os.Create(destPath)
		if err != nil {
			http.Error(w, `{"error": "Failed to save file"}`, http.StatusInternalServerError)
			return
		}
		defer dest.Close()

		if _, err = io.Copy(dest, file); err != nil {
			http.Error(w, `{"error": "Failed to save file contents"}`, http.StatusInternalServerError)
			return
		}

		backups, _ := LoadBackupsList(backupDir)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"backups": backups,
		})
	}
}

// HandleApplyBackup returns an HTTP handler for POST /api/backups/:id/apply.
func HandleApplyBackup(db *sql.DB, configPath string, metadataPath string, reconnect DBReconnectFunc, triggerReload func(), broadcastApplied func()) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := extractBackupID(r.URL.Path)
		id = strings.TrimSuffix(id, "/apply")
		log.Printf("[Go] POST /api/backups/%s/apply", id)

		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		if !isValidBackupID(id) {
			http.Error(w, `{"error": "Invalid backup ID"}`, http.StatusBadRequest)
			return
		}

		backupDir := GetBackupDirPath(r.Context(), db, metadataPath)
		if err := ApplyBackup(r.Context(), configPath, metadataPath, backupDir, id, reconnect); err != nil {
			log.Printf("[Apply Backup] Restore failed: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), http.StatusInternalServerError)
			return
		}

		if triggerReload != nil {
			triggerReload()
		}

		if broadcastApplied != nil {
			broadcastApplied()
		}

		w.WriteHeader(http.StatusOK)
	}
}
