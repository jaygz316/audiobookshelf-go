package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// BackupRestoreMu prevents concurrent backup or restore operations.
var BackupRestoreMu sync.Mutex

// BackupInfo holds details about a single backup file.
type BackupInfo struct {
	ID            string `json:"id"`
	Key           string `json:"key"`
	BackupDirPath string `json:"backupDirPath"`
	DatePretty    string `json:"datePretty"`
	FullPath      string `json:"fullPath"`
	Path          string `json:"path"`
	Filename      string `json:"filename"`
	FileSize      int64  `json:"fileSize"`
	CreatedAt     int64  `json:"createdAt"`
	ServerVersion string `json:"serverVersion"`
}

// ServerSettings is a dependency interface for reading server settings.
type ServerSettings struct {
	BackupPath    string
	BackupsToKeep int
}

// GetBackupDirPath resolves the backup directory from server settings or environment.
func GetBackupDirPath(ctx context.Context, db *sql.DB, metadataPath string) string {
	settings, err := getServerSettings(db)
	if err == nil && settings != nil && settings.BackupPath != "" {
		return settings.BackupPath
	}
	return filepath.Join(metadataPath, "backups")
}

// getServerSettings reads the server settings from the database.
func getServerSettings(db *sql.DB) (*ServerSettings, error) {
	var valStr string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
	if err != nil {
		return nil, err
	}

	var s struct {
		BackupPath    string `json:"backupPath"`
		BackupsToKeep int    `json:"backupsToKeep"`
	}
	if err := json.Unmarshal([]byte(valStr), &s); err != nil {
		return nil, err
	}

	return &ServerSettings{
		BackupPath:    s.BackupPath,
		BackupsToKeep: s.BackupsToKeep,
	}, nil
}

func isValidBackupID(id string) bool {
	if id == "" {
		return false
	}
	// Reject ID parameters containing "..", "/", or "\" to prevent path traversal
	if strings.Contains(id, "..") || strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return false
	}
	return true
}

func extractBackupID(path string) string {
	idx := strings.Index(path, "/api/backups/")
	if idx != -1 {
		return path[idx+len("/api/backups/"):]
	}
	return strings.TrimPrefix(path, "/")
}

func isSafePath(baseDir, targetPath string) bool {
	normalizedBase := filepath.ToSlash(filepath.Clean(baseDir))
	normalizedTarget := filepath.ToSlash(filepath.Clean(targetPath))

	if normalizedTarget == normalizedBase {
		return true
	}

	if !strings.HasSuffix(normalizedBase, "/") {
		normalizedBase += "/"
	}

	return strings.HasPrefix(normalizedTarget, normalizedBase)
}

func timeToDBStr(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05.000 +00:00")
}
