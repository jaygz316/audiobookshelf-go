package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"audiobookshelf/internal/core"
)

type ChapterInfo struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Title string  `json:"title"`
}

func handleEmbedMetadata(db *sql.DB, cfg *core.Config, itemID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /api/items/%s/embed-metadata", itemID)

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

		var mediaID, mediaType, authorName string
		err := db.QueryRow("SELECT mediaId, mediaType, authorNamesFirstLast FROM libraryItems WHERE id = ?", itemID).Scan(&mediaID, &mediaType, &authorName)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, `{"error": "Item not found"}`, http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf(`{"error": "DB error: %v"}`, err), http.StatusInternalServerError)
			return
		}

		if mediaType != "book" {
			http.Error(w, `{"error": "Only books support metadata tag embedding"}`, http.StatusBadRequest)
			return
		}

		// Retrieve all metadata for the book
		var (
			title, subtitle, publishedYear, publishedDate, publisher, description, coverPath string
			bNarrators, bAudioFiles, bChapters, bGenres, bTags                               []byte
		)

		err = db.QueryRow(`
			SELECT title, subtitle, publishedYear, publishedDate, publisher, description, coverPath, narrators, audioFiles, chapters, genres, tags
			FROM books WHERE id = ?`, mediaID).Scan(
			&title, &subtitle, &publishedYear, &publishedDate, &publisher, &description, &coverPath,
			&bNarrators, &bAudioFiles, &bChapters, &bGenres, &bTags,
		)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "DB error querying book: %v"}`, err), http.StatusInternalServerError)
			return
		}

		var narrators []string
		_ = json.Unmarshal(bNarrators, &narrators)

		var audioFiles []map[string]interface{}
		_ = json.Unmarshal(bAudioFiles, &audioFiles)

		var chapters []ChapterInfo
		_ = json.Unmarshal(bChapters, &chapters)

		var genres []string
		_ = json.Unmarshal(bGenres, &genres)

		var tags []string
		_ = json.Unmarshal(bTags, &tags)

		if len(audioFiles) == 0 {
			http.Error(w, `{"error": "No audio files found for this library item"}`, http.StatusBadRequest)
			return
		}

		// Prepare common FFMETADATA info
		var metadataBuf bytes.Buffer
		metadataBuf.WriteString(";FFMETADATA1\n")
		metadataBuf.WriteString(fmt.Sprintf("title=%s\n", escapeMetadataValue(title)))
		metadataBuf.WriteString(fmt.Sprintf("artist=%s\n", escapeMetadataValue(authorName)))
		metadataBuf.WriteString(fmt.Sprintf("album=%s\n", escapeMetadataValue(title)))
		metadataBuf.WriteString(fmt.Sprintf("album_artist=%s\n", escapeMetadataValue(authorName)))

		if len(narrators) > 0 {
			metadataBuf.WriteString(fmt.Sprintf("composer=%s\n", escapeMetadataValue(strings.Join(narrators, ", "))))
		}
		if description != "" {
			metadataBuf.WriteString(fmt.Sprintf("description=%s\n", escapeMetadataValue(description)))
			metadataBuf.WriteString(fmt.Sprintf("comment=%s\n", escapeMetadataValue(description)))
		}
		if publisher != "" {
			metadataBuf.WriteString(fmt.Sprintf("publisher=%s\n", escapeMetadataValue(publisher)))
		}
		dateStr := publishedYear
		if publishedDate != "" {
			dateStr = publishedDate
		}
		if dateStr != "" {
			metadataBuf.WriteString(fmt.Sprintf("date=%s\n", escapeMetadataValue(dateStr)))
		}
		allGenresAndTags := append(genres, tags...)
		if len(allGenresAndTags) > 0 {
			metadataBuf.WriteString(fmt.Sprintf("genre=%s\n", escapeMetadataValue(strings.Join(allGenresAndTags, ", "))))
		}

		// Write chapters
		for _, c := range chapters {
			metadataBuf.WriteString("\n[CHAPTER]\n")
			metadataBuf.WriteString("TIMEBASE=1/1000\n")
			metadataBuf.WriteString(fmt.Sprintf("START=%d\n", int64(c.Start*1000)))
			metadataBuf.WriteString(fmt.Sprintf("END=%d\n", int64(c.End*1000)))
			metadataBuf.WriteString(fmt.Sprintf("title=%s\n", escapeMetadataValue(c.Title)))
		}

		// Create a temporary FFMETADATA file
		metaFile, err := os.CreateTemp("", "ffmetadata-*.txt")
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Failed to create temp metadata file: %v"}`, err), http.StatusInternalServerError)
			return
		}
		defer os.Remove(metaFile.Name())
		defer metaFile.Close()

		if _, err := metaFile.Write(metadataBuf.Bytes()); err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Failed to write temp metadata: %v"}`, err), http.StatusInternalServerError)
			return
		}
		metaFile.Close()

		// Validate cover art path if present
		hasCover := false
		resolvedCoverPath := ""
		if coverPath != "" {
			if _, err := os.Stat(coverPath); err == nil {
				hasCover = true
				resolvedCoverPath = coverPath
			}
		}

		// Process each audio file
		var updatedFiles []string
		for _, af := range audioFiles {
			afMeta, ok := af["metadata"].(map[string]interface{})
			if !ok {
				continue
			}
			filePathVal, ok := afMeta["path"].(string)
			if !ok || filePathVal == "" {
				continue
			}

			if _, err := os.Stat(filePathVal); os.IsNotExist(err) {
				log.Printf("[Warning] Audio file path not found: %s", filePathVal)
				continue
			}

			ext := strings.ToLower(filepath.Ext(filePathVal))
			tmpOutPath := filePathVal + ".embed" + ext

			// Construct ffmpeg command
			// e.g. ffmpeg -y -i input -i metadata.txt [-i cover.jpg] -map_metadata 1 -map 0:a [-map 2:v] -c copy [format specific flags] output
			args := []string{"-y", "-i", filePathVal, "-i", metaFile.Name()}
			if hasCover {
				args = append(args, "-i", resolvedCoverPath)
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
				log.Printf("[Error] FFmpeg execution failed for %s: %v, stderr: %s", filePathVal, err, stderr.String())
				// Attempt cleanup and abort/return error
				os.Remove(tmpOutPath)
				http.Error(w, fmt.Sprintf(`{"error": "FFmpeg failed for file %s: %v. Stderr: %s"}`, filepath.Base(filePathVal), err, stderr.String()), http.StatusInternalServerError)
				return
			}

			// Validate generated file size to avoid truncation
			tmpStat, err := os.Stat(tmpOutPath)
			if err != nil || tmpStat.Size() < 1024 {
				log.Printf("[Error] Embedded output file is missing or too small: %s", tmpOutPath)
				os.Remove(tmpOutPath)
				http.Error(w, fmt.Sprintf(`{"error": "Ffmpeg produced an empty or corrupted file for %s"}`, filepath.Base(filePathVal)), http.StatusInternalServerError)
				return
			}

			// Replace original file with the new file
			if err := os.Rename(tmpOutPath, filePathVal); err != nil {
				// Fallback: copy content
				if errCopy := copyFile(tmpOutPath, filePathVal); errCopy != nil {
					log.Printf("[Error] Failed to overwrite original file: %v", errCopy)
					os.Remove(tmpOutPath)
					http.Error(w, fmt.Sprintf(`{"error": "Failed to overwrite file %s: %v"}`, filepath.Base(filePathVal), errCopy), http.StatusInternalServerError)
					return
				}
				os.Remove(tmpOutPath)
			}

			updatedFiles = append(updatedFiles, filepath.Base(filePathVal))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":      true,
			"message":      "Metadata, chapters, and cover art embedded successfully",
			"updatedFiles": updatedFiles,
		})
	}
}

// Helpers
func escapeMetadataValue(val string) string {
	val = strings.ReplaceAll(val, "\\", "\\\\")
	val = strings.ReplaceAll(val, "=", "\\=")
	val = strings.ReplaceAll(val, ";", "\\;")
	val = strings.ReplaceAll(val, "#", "\\#")
	val = strings.ReplaceAll(val, "\n", "\\\n")
	return val
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
