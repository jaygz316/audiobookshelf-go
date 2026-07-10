package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"audiobookshelf/internal/core"
)

type MergeChapter struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Title string  `json:"title"`
}

type MergeAudioFile struct {
	Index       int     `json:"index"`
	Exclude     bool    `json:"exclude"`
	Duration    float64 `json:"duration"`
	Codec       string  `json:"codec"`
	MimeType    string  `json:"mimeType"`
	StartOffset float64 `json:"startOffset"`
	Title       string  `json:"title"`
	Metadata    struct {
		Path     string `json:"path"`
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
	} `json:"metadata"`
}


func escapeConcatPath(path string) string {
	return strings.ReplaceAll(path, "'", "'\\''")
}

func handleMergeAudioFiles(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Enforce POST method
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		// Authenticate and check permissions
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

		// Retrieve itemID from URL
		// Example: /api/items/some-id/merge
		path := r.URL.Path
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) < 3 || parts[len(parts)-1] != "merge" {
			http.Error(w, `{"error": "Invalid request path"}`, http.StatusBadRequest)
			return
		}
		itemID := parts[len(parts)-2]

		log.Printf("[Go] POST /api/items/%s/merge", itemID)

		// Fetch mediaId, mediaType, and item details
		var mediaID, mediaType, itemPath string
		var authorName sql.NullString
		err := db.QueryRow("SELECT mediaId, mediaType, authorNamesFirstLast, path FROM libraryItems WHERE id = ?", itemID).Scan(&mediaID, &mediaType, &authorName, &itemPath)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, `{"error": "Library item not found"}`, http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf(`{"error": "DB error: %v"}`, err), http.StatusInternalServerError)
			return
		}

		if mediaType != "book" {
			http.Error(w, `{"error": "Only books support audio track merging"}`, http.StatusBadRequest)
			return
		}

		// Query book details
		var title string
		var bAudioFiles []byte
		err = db.QueryRow("SELECT title, audioFiles FROM books WHERE id = ?", mediaID).Scan(&title, &bAudioFiles)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "DB error querying book: %v"}`, err), http.StatusInternalServerError)
			return
		}

		var audioFiles []MergeAudioFile
		_ = json.Unmarshal(bAudioFiles, &audioFiles)

		// Filter active (non-excluded) audio files
		var activeFiles []MergeAudioFile
		for _, af := range audioFiles {
			if !af.Exclude && af.Metadata.Path != "" {
				activeFiles = append(activeFiles, af)
			}
		}

		if len(activeFiles) < 2 {
			http.Error(w, `{"error": "Book must have at least 2 active audio files to merge"}`, http.StatusBadRequest)
			return
		}

		// Verify all active files exist on disk
		for _, af := range activeFiles {
			if _, err := os.Stat(af.Metadata.Path); os.IsNotExist(err) {
				http.Error(w, fmt.Sprintf(`{"error": "Audio file not found on disk: %s"}`, filepath.Base(af.Metadata.Path)), http.StatusBadRequest)
				return
			}
		}

		// Choose the first file directory as the output target directory
		targetDir := filepath.Dir(activeFiles[0].Metadata.Path)
		outputFilename := fmt.Sprintf("%s_merged.m4b", sanitizeFilename(title))
		outputPath := filepath.Join(targetDir, outputFilename)

		// Create a temporary concat file
		concatFile, err := os.CreateTemp("", "ffmpeg-concat-*.txt")
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Failed to create temporary concat file: %v"}`, err), http.StatusInternalServerError)
			return
		}
		defer os.Remove(concatFile.Name())
		defer concatFile.Close()

		for _, af := range activeFiles {
			_, _ = concatFile.WriteString(fmt.Sprintf("file '%s'\n", escapeConcatPath(af.Metadata.Path)))
		}
		_ = concatFile.Close()

		// Generate chapters and ffmetadata
		var chapters []MergeChapter
		var currentOffset float64 = 0.0
		for i, af := range activeFiles {
			chTitle := af.Title
			if chTitle == "" {
				// Strip extension for clean title
				filename := af.Metadata.Filename
				ext := filepath.Ext(filename)
				chTitle = strings.TrimSuffix(filename, ext)
			}

			chapters = append(chapters, MergeChapter{
				ID:    i + 1,
				Start: currentOffset,
				End:   currentOffset + af.Duration,
				Title: chTitle,
			})
			currentOffset += af.Duration
		}

		// Create ffmetadata file for chapters
		metadataBuf := bytes.Buffer{}
		metadataBuf.WriteString(";FFMETADATA1\n")
		metadataBuf.WriteString(fmt.Sprintf("title=%s\n", escapeMetadataValue(title)))
		authorNameStr := ""
		if authorName.Valid {
			authorNameStr = authorName.String
		}
		metadataBuf.WriteString(fmt.Sprintf("artist=%s\n", escapeMetadataValue(authorNameStr)))
		metadataBuf.WriteString(fmt.Sprintf("album=%s\n", escapeMetadataValue(title)))

		for _, c := range chapters {
			metadataBuf.WriteString("\n[CHAPTER]\n")
			metadataBuf.WriteString("TIMEBASE=1/1000\n")
			metadataBuf.WriteString(fmt.Sprintf("START=%d\n", int64(c.Start*1000)))
			metadataBuf.WriteString(fmt.Sprintf("END=%d\n", int64(c.End*1000)))
			metadataBuf.WriteString(fmt.Sprintf("title=%s\n", escapeMetadataValue(c.Title)))
		}

		metadataFile, err := os.CreateTemp("", "ffmpeg-metadata-*.txt")
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Failed to create temporary metadata file: %v"}`, err), http.StatusInternalServerError)
			return
		}
		defer os.Remove(metadataFile.Name())
		defer metadataFile.Close()

		_, _ = metadataFile.Write(metadataBuf.Bytes())
		_ = metadataFile.Close()

		// Determine codec / encoding settings
		// If the first file is already M4B or AAC codec/container, we can perform lossless concat.
		// Otherwise we transcode to AAC.
		firstExt := strings.ToLower(filepath.Ext(activeFiles[0].Metadata.Path))
		useCopy := firstExt == ".m4b" || firstExt == ".m4a" || firstExt == ".mp4"

		// Construct ffmpeg command
		// ffmpeg -y -f concat -safe 0 -i concat.txt -i metadata.txt -map_metadata 1 -map 0:a [encoding/copy flags] output.m4b
		args := []string{
			"-y",
			"-f", "concat",
			"-safe", "0",
			"-i", concatFile.Name(),
			"-i", metadataFile.Name(),
			"-map_metadata", "1",
			"-map", "0:a",
		}

		if useCopy {
			args = append(args, "-c", "copy")
		} else {
			args = append(args, "-c:a", "aac", "-b:a", "128k")
		}
		args = append(args, outputPath)

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
		defer cancel()

		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		log.Printf("[Go] Running ffmpeg command: ffmpeg %s", strings.Join(args, " "))
		if err := cmd.Run(); err != nil {
			log.Printf("[Error] FFmpeg merge execution failed: %v, stderr: %s", err, stderr.String())
			os.Remove(outputPath)
			http.Error(w, fmt.Sprintf(`{"error": "FFmpeg merge execution failed: %v. Stderr: %s"}`, err, stderr.String()), http.StatusInternalServerError)
			return
		}

		// Verify merged file exists and is not empty
		mergedStat, err := os.Stat(outputPath)
		if err != nil || mergedStat.Size() < 1024 {
			log.Printf("[Error] Merged file missing or too small")
			os.Remove(outputPath)
			http.Error(w, `{"error": "Merged file missing or too small"}`, http.StatusInternalServerError)
			return
		}

		// Delete the original audio files
		for _, af := range activeFiles {
			if err := os.Remove(af.Metadata.Path); err != nil {
				log.Printf("[Warning] Failed to delete original file %s: %v", af.Metadata.Path, err)
			}
		}

		// Build updated single track audioFiles JSON
		var updatedAudioFiles []MergeAudioFile
		var mergedTrack MergeAudioFile
		mergedTrack.Index = 0
		mergedTrack.Exclude = false
		mergedTrack.Duration = currentOffset
		mergedTrack.Codec = "aac"
		if useCopy {
			mergedTrack.Codec = activeFiles[0].Codec
			if mergedTrack.Codec == "" {
				mergedTrack.Codec = "aac"
			}
		}
		mergedTrack.MimeType = "audio/mp4"
		mergedTrack.StartOffset = 0.0
		mergedTrack.Title = "Merged Audiobook"
		mergedTrack.Metadata.Path = outputPath
		mergedTrack.Metadata.Filename = outputFilename
		mergedTrack.Metadata.Size = mergedStat.Size()

		updatedAudioFiles = append(updatedAudioFiles, mergedTrack)

		audioFilesJSON, err := json.Marshal(updatedAudioFiles)
		if err != nil {
			http.Error(w, "Failed to marshal new audioFiles array: "+err.Error(), http.StatusInternalServerError)
			return
		}

		chaptersJSON, err := json.Marshal(chapters)
		if err != nil {
			http.Error(w, "Failed to marshal chapters: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Update database
		tx, err := db.Begin()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		_, err = tx.Exec("UPDATE books SET audioFiles = ?, chapters = ?, duration = ? WHERE id = ?", audioFilesJSON, chaptersJSON, currentOffset, mediaID)
		if err != nil {
			http.Error(w, "Failed to update book in DB: "+err.Error(), http.StatusInternalServerError)
			return
		}

		nowStr := time.Now().Format("2006-01-02 15:04:05.000")
		_, err = tx.Exec("UPDATE libraryItems SET size = ?, updatedAt = ? WHERE id = ?", mergedStat.Size(), nowStr, itemID)
		if err != nil {
			http.Error(w, "Failed to update library item size/timestamp in DB: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			http.Error(w, "Failed to commit transaction: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Audio files merged successfully into a single M4B file.",
		})
	}
}
