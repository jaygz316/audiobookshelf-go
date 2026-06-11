//go:build ignore
// +build ignore

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	_ "modernc.org/sqlite"
)

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nameToLastFirst(name string) string {
	parts := strings.Fields(name)
	if len(parts) > 1 {
		return parts[len(parts)-1] + ", " + strings.Join(parts[:len(parts)-1], " ")
	}
	return name
}

func main() {
	db, err := sql.Open("sqlite", "config/absdatabase.sqlite")
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	itemID := "e53c394e-3d9b-40b8-b164-ea236955257c"

	var id, ino, libraryID, folderID, path, relPath, mediaType, mediaID, mtimeStr, ctimeStr, birthtimeStr, createdAtStr, updatedAtStr string
	var isFileVal, isMissingVal, isInvalidVal int
	var size int64

	query := `
		SELECT id, ino, libraryId, libraryFolderId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size
		FROM libraryItems
		WHERE id = ?
	`
	err = db.QueryRow(query, itemID).Scan(
		&id, &ino, &libraryID, &folderID, &path, &relPath, &isFileVal, &mtimeStr, &ctimeStr, &birthtimeStr, &createdAtStr, &updatedAtStr, &isMissingVal, &isInvalidVal, &mediaType, &mediaID, &size,
	)
	if err != nil {
		log.Fatalf("scan libraryItems: %v", err)
	}

	payload := map[string]interface{}{
		"id":           id,
		"ino":          ino,
		"libraryId":    libraryID,
		"folderId":     folderID,
		"path":         path,
		"relPath":      relPath,
		"isFile":       isFileVal != 0,
		"isMissing":    isMissingVal != 0,
		"isInvalid":    isInvalidVal != 0,
		"mediaType":    mediaType,
		"size":         size,
		"libraryFiles": []interface{}{},
	}

	if mediaType == "book" {
		var bTitle string
		var bTitleIgnorePrefix, bSubtitle, bPublishedYear, bPublishedDate, bPublisher, bDescription, bIsbn, bAsin, bLanguage, bCoverPath sql.NullString
		var bDuration float64
		var bNarrators, bAudioFiles, bEbookFile, bChapters, bTags, bGenres []byte
		var bExplicit, bAbridged sql.NullInt64

		err = db.QueryRow(`
			SELECT title, titleIgnorePrefix, subtitle, publishedYear, publishedDate, publisher, description, isbn, asin, language, explicit, abridged, coverPath, duration, narrators, audioFiles, ebookFile, chapters, tags, genres
			FROM books WHERE id = ?
		`, mediaID).Scan(
			&bTitle, &bTitleIgnorePrefix, &bSubtitle, &bPublishedYear, &bPublishedDate, &bPublisher, &bDescription, &bIsbn, &bAsin, &bLanguage, &bExplicit, &bAbridged, &bCoverPath, &bDuration, &bNarrators, &bAudioFiles, &bEbookFile, &bChapters, &bTags, &bGenres,
		)
		if err != nil {
			log.Fatalf("scan books: %v", err)
		}

		var tags []string
		_ = json.Unmarshal(bTags, &tags)
		var genres []string
		_ = json.Unmarshal(bGenres, &genres)
		var audioFiles []map[string]interface{}
		_ = json.Unmarshal(bAudioFiles, &audioFiles)
		var ebook interface{}
		_ = json.Unmarshal(bEbookFile, &ebook)
		var chapters []interface{}
		_ = json.Unmarshal(bChapters, &chapters)

		var authorNames []string
		var seriesNames []string
		var narratorNames []string
		_ = json.Unmarshal(bNarrators, &narratorNames)

		var authorsList []map[string]interface{} = []map[string]interface{}{}
		rows, err := db.Query("SELECT id, name FROM authors WHERE id IN (SELECT authorId FROM bookAuthors WHERE bookId = ?)", mediaID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var authorID, name string
				if err := rows.Scan(&authorID, &name); err == nil {
					authorsList = append(authorsList, map[string]interface{}{
						"id":   authorID,
						"name": name,
					})
					authorNames = append(authorNames, name)
				}
			}
		}

		var seriesList []map[string]interface{} = []map[string]interface{}{}
		srows, err := db.Query("SELECT s.id, s.name, bs.sequence FROM series s JOIN bookSeries bs ON s.id = bs.seriesId WHERE bs.bookId = ?", mediaID)
		if err == nil {
			defer srows.Close()
			for srows.Next() {
				var seriesID, name, sequence string
				if err := srows.Scan(&seriesID, &name, &sequence); err == nil {
					seriesList = append(seriesList, map[string]interface{}{
						"id":       seriesID,
						"name":     name,
						"sequence": sequence,
					})
					seriesNames = append(seriesNames, name)
				}
			}
		}

		authorName := strings.Join(authorNames, ", ")
		seriesName := strings.Join(seriesNames, ", ")
		narratorName := strings.Join(narratorNames, ", ")

		var ebookFormat *string
		if len(bEbookFile) > 0 {
			var eb struct {
				EbookFormat string `json:"ebookFormat"`
			}
			if json.Unmarshal(bEbookFile, &eb) == nil && eb.EbookFormat != "" {
				ebookFormat = &eb.EbookFormat
			}
		}

		type AudiobookTrack struct {
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

		var rawTracks []AudiobookTrack
		_ = json.Unmarshal(bAudioFiles, &rawTracks)

		var tracks []map[string]interface{}
		var currentOffset float64 = 0.0
		for _, rt := range rawTracks {
			if rt.Exclude {
				continue
			}
			title := rt.Title
			if title == "" {
				title = rt.Metadata.Filename
			}
			tracks = append(tracks, map[string]interface{}{
				"index":       rt.Index,
				"startOffset": currentOffset,
				"duration":    rt.Duration,
				"title":       title,
				"mimeType":    rt.MimeType,
				"metadata": map[string]interface{}{
					"path":     rt.Metadata.Path,
					"filename": rt.Metadata.Filename,
					"size":     rt.Metadata.Size,
				},
			})
			currentOffset += rt.Duration
		}

		payload["media"] = map[string]interface{}{
			"id":            mediaID,
			"coverPath":     nullIfEmpty(bCoverPath.String),
			"tags":          tags,
			"numTracks":     len(tracks),
			"numAudioFiles": len(audioFiles),
			"numChapters":   len(chapters),
			"duration":      bDuration,
			"size":          size,
			"ebookFormat":   ebookFormat,
			"audioFiles":    audioFiles,
			"tracks":        tracks,
			"ebookFile":     ebook,
			"chapters":      chapters,
			"metadata": map[string]interface{}{
				"title":             bTitle,
				"titleIgnorePrefix": bTitleIgnorePrefix.String,
				"subtitle":          nullIfEmpty(bSubtitle.String),
				"authors":           authorsList,
				"authorName":        authorName,
				"authorNameLF":      nameToLastFirst(authorName),
				"narrators":         narratorNames,
				"narratorName":      narratorName,
				"series":            seriesList,
				"seriesName":        seriesName,
				"genres":            genres,
				"publishedYear":     nullIfEmpty(bPublishedYear.String),
				"publishedDate":     nullIfEmpty(bPublishedDate.String),
				"publisher":         nullIfEmpty(bPublisher.String),
				"description":       nullIfEmpty(bDescription.String),
				"isbn":              nullIfEmpty(bIsbn.String),
				"asin":              nullIfEmpty(bAsin.String),
				"language":          nullIfEmpty(bLanguage.String),
				"explicit":          bExplicit.Valid && bExplicit.Int64 != 0,
				"abridged":          bAbridged.Valid && bAbridged.Int64 != 0,
			},
		}
	}

	payloadBytes, _ := json.MarshalIndent(payload, "", "  ")
	fmt.Println(string(payloadBytes))
}
