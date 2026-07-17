package handlers

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"audiobookshelf/internal/core"
	log "audiobookshelf/internal/logger"
	"audiobookshelf/internal/utils"
)

func embedMetadataInAudioFiles(db *sql.DB, cfg *core.Config, audioFiles []map[string]interface{}, metaFilePath string, coverPath string, hasCover bool) ([]string, error) {
	var updatedFiles []string
	for _, af := range audioFiles {
		updatedName, err := embedMetadataInAudioFile(db, cfg, af, metaFilePath, coverPath, hasCover)
		if err != nil {
			return nil, err
		}
		if updatedName != "" {
			updatedFiles = append(updatedFiles, updatedName)
		}
	}
	return updatedFiles, nil
}

func embedMetadataInAudioFile(db *sql.DB, cfg *core.Config, af map[string]interface{}, metaFilePath string, coverPath string, hasCover bool) (string, error) {
	afMeta, ok := af["metadata"].(map[string]interface{})
	if !ok {
		return "", nil
	}
	filePathVal, ok := afMeta["path"].(string)
	if !ok || filePathVal == "" {
		return "", nil
	}

	if !utils.IsSafeFilePath(db, cfg.MetadataPath, filePathVal) {
		log.Warnf("[EmbedMetadata] Path traversal blocked or unsafe file path: %s", filePathVal)
		return "", nil
	}

	if _, err := os.Stat(filePathVal); os.IsNotExist(err) {
		log.Warnf("[Warning] Audio file path not found: %s", filePathVal)
		return "", nil
	}

	ext := strings.ToLower(filepath.Ext(filePathVal))
	tmpOutPath := filePathVal + ".embed" + ext

	// Construct ffmpeg command
	args := []string{"-y", "-i", filePathVal, "-i", metaFilePath}
	if hasCover {
		args = append(args, "-i", coverPath)
	}

	args = append(args, "-map_metadata", "1", "-map", "0:a")
	if hasCover {
		args = append(args, "-map", "2:v")
	}

	args = append(args, "-c", "copy")

	if ext == ".mp3" {
		args = append(args, "-id3v2_version", "3")
		if hasCover {
			args = append(args, "-metadata:s:v", "title=Album cover", "-metadata:s:v", "comment=Cover (front)")
		}
	} else if ext == ".m4b" || ext == ".m4a" || ext == ".mp4" {
		if hasCover {
			args = append(args, "-disposition:v", "attached_pic")
		}
	}

	args = append(args, tmpOutPath)

	cmd := exec.Command("ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Errorf("[Error] FFmpeg execution failed for %s: %v, stderr: %s", filePathVal, err, stderr.String())
		os.Remove(tmpOutPath)
		return "", fmt.Errorf("FFmpeg failed for file %s: %v. Stderr: %s", filepath.Base(filePathVal), err, stderr.String())
	}

	// Validate generated file size to avoid truncation
	tmpStat, err := os.Stat(tmpOutPath)
	if err != nil || tmpStat.Size() < 1024 {
		log.Errorf("[Error] Embedded output file is missing or too small: %s", tmpOutPath)
		os.Remove(tmpOutPath)
		return "", fmt.Errorf("Ffmpeg produced an empty or corrupted file for %s", filepath.Base(filePathVal))
	}

	// Replace original file with the new file
	if err := os.Rename(tmpOutPath, filePathVal); err != nil {
		// Fallback: copy content
		if errCopy := copyFile(tmpOutPath, filePathVal); errCopy != nil {
			log.Errorf("[Error] Failed to overwrite original file: %v", errCopy)
			os.Remove(tmpOutPath)
			return "", fmt.Errorf("Failed to overwrite file %s: %v", filepath.Base(filePathVal), errCopy)
		}
		os.Remove(tmpOutPath)
	}

	return filepath.Base(filePathVal), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Sync()
}
