package handlers

import (
	"database/sql"
	"encoding/json"
)

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

func getAuthorsList(db *sql.DB, mediaID string) ([]map[string]interface{}, []string, error) {
	var authorsList []map[string]interface{} = []map[string]interface{}{}
	var authorNames []string
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
	return authorsList, authorNames, err
}

func getSeriesList(db *sql.DB, mediaID string) ([]map[string]interface{}, []string, error) {
	var seriesList []map[string]interface{} = []map[string]interface{}{}
	var seriesNames []string
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
	return seriesList, seriesNames, err
}

func processAudiobookTracks(bAudioFiles []byte) ([]map[string]interface{}, float64) {
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
	return tracks, currentOffset
}

func buildLibraryFilesForBook(bEbookFile []byte, audioFiles []map[string]interface{}, ino string) []interface{} {
	var libraryFiles []interface{}

	if len(bEbookFile) > 0 {
		var eb struct {
			Metadata struct {
				Filename string `json:"filename"`
				Ext      string `json:"ext"`
				Path     string `json:"path"`
				RelPath  string `json:"relPath"`
				Size     int64  `json:"size"`
				Ctime    int64  `json:"ctime"`
				Mtime    int64  `json:"mtime"`
			} `json:"metadata"`
		}
		if json.Unmarshal(bEbookFile, &eb) == nil && eb.Metadata.Filename != "" {
			libraryFiles = append(libraryFiles, map[string]interface{}{
				"ino":      ino,
				"filename": eb.Metadata.Filename,
				"ext":      eb.Metadata.Ext,
				"path":     eb.Metadata.Path,
				"relPath":  eb.Metadata.RelPath,
				"size":     eb.Metadata.Size,
				"fileType": "ebook",
				"mtime":    eb.Metadata.Mtime,
				"ctime":    eb.Metadata.Ctime,
			})
		}
	}

	for _, af := range audioFiles {
		lfItem := map[string]interface{}{
			"fileType": "audio",
		}
		if val, ok := af["ino"]; ok {
			lfItem["ino"] = val
		}
		if val, ok := af["filename"]; ok {
			lfItem["filename"] = val
		}
		if val, ok := af["ext"]; ok {
			lfItem["ext"] = val
		}
		if val, ok := af["size"]; ok {
			lfItem["size"] = val
		}
		if metadata, ok := af["metadata"].(map[string]interface{}); ok {
			if val, ok := metadata["path"]; ok {
				lfItem["path"] = val
			}
			if val, ok := metadata["relPath"]; ok {
				lfItem["relPath"] = val
			}
			if val, ok := metadata["mtime"]; ok {
				lfItem["mtime"] = val
			}
			if val, ok := metadata["ctime"]; ok {
				lfItem["ctime"] = val
			}
		}
		libraryFiles = append(libraryFiles, lfItem)
	}
	return libraryFiles
}

func getOtherBookVersions(db *sql.DB, libraryID, itemID, bTitle string) []map[string]interface{} {
	var otherVersions []map[string]interface{} = []map[string]interface{}{}
	vrows, err := db.Query(`
		SELECT li.id, b.title, b.subtitle, b.narrators, b.duration, b.coverPath
		FROM libraryItems li
		JOIN books b ON li.mediaId = b.id AND li.mediaType = 'book'
		WHERE li.libraryId = ? AND li.id != ? AND LOWER(b.title) = LOWER(?)
	`, libraryID, itemID, bTitle)
	if err == nil {
		defer vrows.Close()
		for vrows.Next() {
			var vID, vTitle string
			var vSubtitle, vCoverPath sql.NullString
			var vNarrators []byte
			var vDuration float64
			if err := vrows.Scan(&vID, &vTitle, &vSubtitle, &vNarrators, &vDuration, &vCoverPath); err == nil {
				var narrators []string
				_ = json.Unmarshal(vNarrators, &narrators)

				otherVersions = append(otherVersions, map[string]interface{}{
					"id":        vID,
					"title":     vTitle,
					"subtitle":  vSubtitle.String,
					"narrators": narrators,
					"duration":  vDuration,
					"coverPath": vCoverPath.String,
				})
			}
		}
	}
	return otherVersions
}
