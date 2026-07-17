package backup

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	log "audiobookshelf/internal/logger"
)

// LoadBackupsList returns the list of backups sorted by newest first.
func LoadBackupsList(backupDir string) ([]BackupInfo, error) {
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, err
	}

	files, err := os.ReadDir(backupDir)
	if err != nil {
		return nil, err
	}

	var backups []BackupInfo
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".audiobookshelf") {
			continue
		}

		fullPath := filepath.Join(backupDir, file.Name())
		info, err := file.Info()
		if err != nil {
			continue
		}

		b, err := ReadBackupDetailsFromZip(fullPath, info.Size())
		if err != nil {
			log.Printf("[Backups] Warning: failed to read zip %s: %v", file.Name(), err)
			continue
		}

		backups = append(backups, b)
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt > backups[j].CreatedAt
	})

	return backups, nil
}

// ReadBackupDetailsFromZip extracts the backup details from the details file in the ZIP.
func ReadBackupDetailsFromZip(zipPath string, fileSize int64) (BackupInfo, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return BackupInfo{}, err
	}
	defer r.Close()

	var detailsContent string
	for _, f := range r.File {
		if f.Name == "details" {
			rc, err := f.Open()
			if err != nil {
				return BackupInfo{}, err
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return BackupInfo{}, err
			}
			detailsContent = string(data)
			break
		}
	}

	return parseBackupDetails(detailsContent, zipPath, fileSize), nil
}

func parseBackupDetails(detailsContent string, zipPath string, fileSize int64) BackupInfo {
	lines := strings.Split(detailsContent, "\n")
	filename := filepath.Base(zipPath)

	if len(lines) < 3 {
		id := strings.TrimSuffix(filename, ".audiobookshelf")
		createdAt := time.Now().UnixNano() / int64(time.Millisecond)
		return BackupInfo{
			ID:            id,
			Key:           "sqlite",
			BackupDirPath: filepath.Dir(zipPath),
			DatePretty:    time.Now().UTC().Format("Mon, Jan _2 2006 15:04"),
			FullPath:      zipPath,
			Path:          filepath.Join("backups", filename),
			Filename:      filename,
			FileSize:      fileSize,
			CreatedAt:     createdAt,
			ServerVersion: "2.8.0",
		}
	}

	id := lines[0]
	key := lines[1]
	var createdAt int64
	fmt.Sscanf(lines[2], "%d", &createdAt)
	serverVersion := "2.8.0"
	if len(lines) > 3 {
		serverVersion = lines[3]
	}

	t := time.Unix(createdAt/1000, 0).UTC()
	return BackupInfo{
		ID:            id,
		Key:           key,
		BackupDirPath: filepath.Dir(zipPath),
		DatePretty:    t.Format("Mon, Jan _2 2006 15:04"),
		FullPath:      zipPath,
		Path:          filepath.Join("backups", filename),
		Filename:      filename,
		FileSize:      fileSize,
		CreatedAt:     createdAt,
		ServerVersion: serverVersion,
	}
}
