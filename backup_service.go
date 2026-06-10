package main

import (
	"archive/zip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

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

// GetBackupDirPath resolves the backup directory from server settings or environment
func GetBackupDirPath(ctx context.Context, db *sql.DB, metadataPath string) string {
	settings, err := GetServerSettings(db)
	if err == nil && settings != nil && settings.BackupPath != "" {
		return settings.BackupPath
	}
	return filepath.Join(metadataPath, "backups")
}

// LoadBackupsList returns the list of backups sorted by newest first
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

// ReadBackupDetailsFromZip extracts the backup details from details file in the ZIP
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

// CreateBackup orchestrates generating database snapshot and metadata archives
func CreateBackup(ctx context.Context, db *sql.DB, configPath, metadataPath, backupDir string) ([]BackupInfo, error) {
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, err
	}

	now := time.Now()
	id := now.Format("2006-01-02T1504")
	fullPath := filepath.Join(backupDir, id+".audiobookshelf")

	tempDBPath, err := createDBSnapshot(ctx, db, configPath, id)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tempDBPath)

	if err := zipBackupContents(fullPath, tempDBPath, metadataPath, id, now); err != nil {
		return nil, err
	}

	if err := pruneOldBackups(db, backupDir); err != nil {
		log.Printf("[Backups] Pruning old backups failed: %v", err)
	}

	return LoadBackupsList(backupDir)
}

func createDBSnapshot(ctx context.Context, db *sql.DB, configPath, id string) (string, error) {
	tempDBPath := filepath.Join(os.TempDir(), fmt.Sprintf("absdatabase-%s.sqlite", id))

	_, err := db.ExecContext(ctx, fmt.Sprintf("VACUUM INTO '%s'", tempDBPath))
	if err == nil {
		return tempDBPath, nil
	}

	log.Printf("[Backups] VACUUM INTO failed: %v. Falling back to simple file copy", err)
	srcFile, err := os.Open(filepath.Join(configPath, "absdatabase.sqlite"))
	if err != nil {
		return "", err
	}
	defer srcFile.Close()

	destFile, err := os.Create(tempDBPath)
	if err != nil {
		return "", err
	}
	defer destFile.Close()

	if _, err = io.Copy(destFile, srcFile); err != nil {
		os.Remove(tempDBPath)
		return "", err
	}

	return tempDBPath, nil
}

func zipBackupContents(destZipPath, tempDBPath, metadataPath, id string, now time.Time) error {
	zipFile, err := os.Create(destZipPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zw := zip.NewWriter(zipFile)
	defer zw.Close()

	if err := addFileToZip(zw, tempDBPath, "absdatabase.sqlite"); err != nil {
		return err
	}

	createdAt := now.UnixNano() / int64(time.Millisecond)
	detailsString := fmt.Sprintf("%s\nsqlite\n%d\n2.8.0\n", id, createdAt)
	writer, err := zw.Create("details")
	if err != nil {
		return err
	}
	if _, err := writer.Write([]byte(detailsString)); err != nil {
		return err
	}

	if err := addDirToZip(zw, filepath.Join(metadataPath, "items"), "metadata-items"); err != nil {
		return err
	}

	if err := addDirToZip(zw, filepath.Join(metadataPath, "authors"), "metadata-authors"); err != nil {
		return err
	}

	// Verify Zip Writer Close error
	if err := zw.Close(); err != nil {
		return err
	}

	return zipFile.Close()
}

func addFileToZip(zw *zip.Writer, srcPath, destName string) error {
	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(fi)
	if err != nil {
		return err
	}
	header.Name = destName
	header.Method = zip.Deflate

	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(writer, file)
	return err
}

func addDirToZip(zw *zip.Writer, srcDir string, zipDirName string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return nil
		}
		// Windows separator normalization: filepath.ToSlash
		header.Name = filepath.ToSlash(filepath.Join(zipDirName, rel))
		header.Method = zip.Deflate

		writer, err := zw.CreateHeader(header)
		if err != nil {
			return nil
		}
		io.Copy(writer, file)
		return nil
	})
}

func pruneOldBackups(db *sql.DB, backupDir string) error {
	settings, err := GetServerSettings(db)
	backupsToKeep := 2
	if err == nil && settings != nil && settings.BackupsToKeep > 0 {
		backupsToKeep = settings.BackupsToKeep
	}

	backups, err := LoadBackupsList(backupDir)
	if err != nil {
		return err
	}

	if len(backups) > backupsToKeep {
		for i := backupsToKeep; i < len(backups); i++ {
			log.Printf("[Backups] Pruning old backup: %s", backups[i].FullPath)
			os.Remove(backups[i].FullPath)
		}
	}
	return nil
}

// ApplyBackup performs low-level backup restore operations on db and metadata directories
func ApplyBackup(ctx context.Context, configPath, metadataPath, backupDir, id string) error {
	fullPath := filepath.Join(backupDir, id+".audiobookshelf")

	// Disconnect DB connection in Go
	if globalDB != nil {
		globalDB.Close()
	}

	if err := restoreDBFile(configPath, fullPath); err != nil {
		reconnectDB(filepath.Join(configPath, "absdatabase.sqlite"))
		return err
	}

	if err := restoreMetadataFiles(metadataPath, fullPath); err != nil {
		reconnectDB(filepath.Join(configPath, "absdatabase.sqlite"))
		return err
	}

	return reconnectDB(filepath.Join(configPath, "absdatabase.sqlite"))
}

func reconnectDB(dbPath string) error {
	dbFile, err := initDB(dbPath)
	if err != nil {
		log.Printf("[Apply Backup] Failed to reconnect to restored DB: %v", err)
		return err
	}
	globalDB = dbFile
	if SocketAuth != nil {
		SocketAuth.db = dbFile
	}
	reinitManagers(dbFile)
	return nil
}

func restoreDBFile(configPath, zipPath string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	var sqliteEntry *zip.File
	for _, f := range zr.File {
		if f.Name == "absdatabase.sqlite" {
			sqliteEntry = f
			break
		}
	}

	if sqliteEntry == nil {
		return fmt.Errorf("backup ZIP missing database file")
	}

	tempDBPath := filepath.Join(configPath, "absdatabase-temp.sqlite")
	tempFile, err := os.Create(tempDBPath)
	if err != nil {
		return err
	}
	defer tempFile.Close()

	rc, err := sqliteEntry.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	if _, err = io.Copy(tempFile, rc); err != nil {
		os.Remove(tempDBPath)
		return err
	}

	tempFile.Close()
	rc.Close()

	realDBPath := filepath.Join(configPath, "absdatabase.sqlite")
	_ = os.Remove(realDBPath)
	if err := os.Rename(tempDBPath, realDBPath); err != nil {
		os.Remove(tempDBPath)
		return err
	}

	return nil
}

func restoreMetadataFiles(metadataPath, zipPath string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	itemsDir := filepath.Join(metadataPath, "items")
	authorsDir := filepath.Join(metadataPath, "authors")

	for _, f := range zr.File {
		var baseDir string
		var rel string

		if strings.HasPrefix(f.Name, "metadata-items/") {
			baseDir = itemsDir
			rel = strings.TrimPrefix(f.Name, "metadata-items/")
		} else if strings.HasPrefix(f.Name, "metadata-authors/") {
			baseDir = authorsDir
			rel = strings.TrimPrefix(f.Name, "metadata-authors/")
		} else {
			continue
		}

		if rel == "" {
			continue
		}

		dest := filepath.Join(baseDir, rel)

		// Zip Slip vulnerability check
		if !isSafePath(baseDir, dest) {
			return fmt.Errorf("zip slip detected: path %s goes outside %s", dest, baseDir)
		}

		if err := extractZipEntry(f, dest); err != nil {
			return err
		}
	}
	return nil
}

func extractZipEntry(f *zip.File, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	frc, err := f.Open()
	if err != nil {
		return err
	}
	defer frc.Close()

	_, err = io.Copy(out, frc)
	return err
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
