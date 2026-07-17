package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"audiobookshelf/internal/utils"
)

// updateDatabaseAndCleanup deletes the source files and updates DB tables.
func updateDatabaseAndCleanup(db *sql.DB, mergeCtx *MergeContext, chapters []MergeChapter, totalDuration float64) (int, error) {
	for _, af := range mergeCtx.ActiveFiles {
		if utils.IsSafeFilePath(db, MetadataPath, af.Metadata.Path) {
			if err := os.Remove(af.Metadata.Path); err != nil {
				log.Warnf("[Warning] Failed to delete original file %s: %v", af.Metadata.Path, err)
			}
		} else {
			log.Warnf("[Warning] Blocked delete of unsafe original file %s", af.Metadata.Path)
		}
	}

	mergedStat, err := os.Stat(mergeCtx.OutputPath)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to stat merged file: %w", err)
	}

	var updatedAudioFiles []MergeAudioFile
	var mergedTrack MergeAudioFile
	mergedTrack.Index = 0
	mergedTrack.Exclude = false
	mergedTrack.Duration = totalDuration
	mergedTrack.Codec = "aac"
	if mergeCtx.UseCopy {
		mergedTrack.Codec = mergeCtx.ActiveFiles[0].Codec
		if mergedTrack.Codec == "" {
			mergedTrack.Codec = "aac"
		}
	}
	mergedTrack.MimeType = "audio/mp4"
	mergedTrack.StartOffset = 0.0
	mergedTrack.Title = "Merged Audiobook"
	mergedTrack.Metadata.Path = mergeCtx.OutputPath
	mergedTrack.Metadata.Filename = mergeCtx.OutputFilename
	mergedTrack.Metadata.Size = mergedStat.Size()

	updatedAudioFiles = append(updatedAudioFiles, mergedTrack)

	audioFilesJSON, err := json.Marshal(updatedAudioFiles)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("Failed to marshal new audioFiles array: %w", err)
	}

	chaptersJSON, err := json.Marshal(chapters)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("Failed to marshal chapters: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec("UPDATE books SET audioFiles = ?, chapters = ?, duration = ? WHERE id = ?", audioFilesJSON, chaptersJSON, totalDuration, mergeCtx.MediaID)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("Failed to update book in DB: %w", err)
	}

	nowStr := time.Now().Format("2006-01-02 15:04:05.000")
	_, err = tx.Exec("UPDATE libraryItems SET size = ?, updatedAt = ? WHERE id = ?", mergedStat.Size(), nowStr, mergeCtx.ItemID)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("Failed to update library item size/timestamp in DB: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return 0, nil
}
