package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"audiobookshelf/internal/utils"
)

// prepareMergeContext validates the item and generates the execution context.
func prepareMergeContext(db *sql.DB, itemID string) (*MergeContext, int, error) {
	if strings.Contains(itemID, "..") || strings.Contains(itemID, "/") || strings.Contains(itemID, "\\") {
		return nil, http.StatusBadRequest, fmt.Errorf("Invalid item ID")
	}

	var mediaID, mediaType, itemPath string
	var authorName sql.NullString
	err := db.QueryRow("SELECT mediaId, mediaType, authorNamesFirstLast, path FROM libraryItems WHERE id = ?", itemID).Scan(&mediaID, &mediaType, &authorName, &itemPath)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, http.StatusNotFound, fmt.Errorf("Library item not found")
		}
		return nil, http.StatusInternalServerError, fmt.Errorf("DB error: %v", err)
	}

	if mediaType != "book" {
		return nil, http.StatusBadRequest, fmt.Errorf("Only books support audio track merging")
	}

	var title string
	var bAudioFiles []byte
	err = db.QueryRow("SELECT title, audioFiles FROM books WHERE id = ?", mediaID).Scan(&title, &bAudioFiles)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("DB error querying book: %v", err)
	}

	var audioFiles []MergeAudioFile
	_ = json.Unmarshal(bAudioFiles, &audioFiles)

	var activeFiles []MergeAudioFile
	for _, af := range audioFiles {
		if !af.Exclude && af.Metadata.Path != "" {
			activeFiles = append(activeFiles, af)
		}
	}

	if len(activeFiles) < 2 {
		return nil, http.StatusBadRequest, fmt.Errorf("Book must have at least 2 active audio files to merge")
	}

	for _, af := range activeFiles {
		if !utils.IsSafeFilePath(db, MetadataPath, af.Metadata.Path) {
			return nil, http.StatusForbidden, fmt.Errorf("Forbidden: unsafe audio file path")
		}
		if _, err := os.Stat(af.Metadata.Path); os.IsNotExist(err) {
			return nil, http.StatusBadRequest, fmt.Errorf("Audio file not found on disk: %s", filepath.Base(af.Metadata.Path))
		}
	}

	targetDir := filepath.Dir(activeFiles[0].Metadata.Path)
	outputFilename := fmt.Sprintf("%s_merged.m4b", sanitizeFilename(title))
	outputPath := filepath.Join(targetDir, outputFilename)
	if !utils.IsSafeFilePath(db, MetadataPath, outputPath) {
		return nil, http.StatusForbidden, fmt.Errorf("Forbidden: unsafe output path")
	}

	firstExt := strings.ToLower(filepath.Ext(activeFiles[0].Metadata.Path))
	useCopy := firstExt == ".m4b" || firstExt == ".m4a" || firstExt == ".mp4"

	return &MergeContext{
		ItemID:         itemID,
		MediaID:        mediaID,
		Title:          title,
		AuthorName:     authorName,
		ActiveFiles:    activeFiles,
		OutputPath:     outputPath,
		OutputFilename: outputFilename,
		UseCopy:        useCopy,
	}, 0, nil
}
