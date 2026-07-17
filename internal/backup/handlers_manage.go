package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"audiobookshelf/internal/core"
	log "audiobookshelf/internal/logger"
)

// HandleGetBackups returns an HTTP handler for GET /api/backups.
func HandleGetBackups(db *sql.DB, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/backups")
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		backupDir := GetBackupDirPath(r.Context(), db, metadataPath)
		backups, err := LoadBackupsList(backupDir)
		if err != nil {
			log.Printf("[Backups] Failed to load list: %v", err)
			http.Error(w, `{"error": "Failed to load backups"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"backups":          backups,
			"backupLocation":   backupDir,
			"backupPathEnvSet": os.Getenv("BACKUP_PATH") != "",
		})
	}
}

// HandleDeleteBackup returns an HTTP handler for DELETE /api/backups/:id.
func HandleDeleteBackup(db *sql.DB, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := extractBackupID(r.URL.Path)
		log.Printf("[Go] DELETE /api/backups/%s", id)

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
		filename := id + ".audiobookshelf"
		fullPath := filepath.Join(backupDir, filename)

		if err := os.Remove(fullPath); err != nil {
			log.Printf("[Backups] Delete failed: %v", err)
			http.Error(w, `{"error": "Failed to delete backup"}`, http.StatusNotFound)
			return
		}

		backups, _ := LoadBackupsList(backupDir)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"backups": backups,
		})
	}
}

// HandleDownloadBackup returns an HTTP handler for GET /api/backups/:id/download.
func HandleDownloadBackup(db *sql.DB, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := extractBackupID(r.URL.Path)
		id = strings.TrimSuffix(id, "/download")
		log.Printf("[Go] GET /api/backups/%s/download", id)

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
		filename := id + ".audiobookshelf"
		fullPath := filepath.Join(backupDir, filename)

		w.Header().Set("Content-Disposition", "attachment; filename="+filename)
		http.ServeFile(w, r, fullPath)
	}
}

// HandleUpdateBackupPath returns an HTTP handler for PATCH /api/backups/path.
func HandleUpdateBackupPath(db *sql.DB, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] PATCH /api/backups/path")
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var body struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
			return
		}

		newPath := filepath.Clean(body.Path)
		if err := os.MkdirAll(newPath, 0755); err != nil {
			log.Printf("[Backups] Failed to create path: %v", err)
			http.Error(w, `{"error": "Invalid path"}`, http.StatusBadRequest)
			return
		}

		if err := updateBackupPathInSettings(r.Context(), db, newPath); err != nil {
			http.Error(w, `{"error": "Failed to update backup settings"}`, http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func updateBackupPathInSettings(ctx context.Context, db *sql.DB, newPath string) error {
	var valStr string
	err := db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
	currentSettings := make(map[string]interface{})
	if err == nil && valStr != "" {
		json.Unmarshal([]byte(valStr), &currentSettings)
	}

	currentSettings["backupPath"] = newPath
	newValBytes, _ := json.Marshal(currentSettings)
	nowStr := timeToDBStr(time.Now())

	_, err = db.ExecContext(ctx, "INSERT INTO settings (key, value, createdAt, updatedAt) VALUES ('server-settings', ?, ?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updatedAt=excluded.updatedAt",
		string(newValBytes), nowStr, nowStr)
	return err
}
