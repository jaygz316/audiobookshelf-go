package handlers

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"

	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/utils"
)

func writeBatchMetadata(db *sql.DB, itemID string, mediaID string, mediaType string, srvSettings *idb.ServerSettings) error {
	var metadataPath string
	var dbErr error
	if srvSettings != nil && srvSettings.MetadataMarkdownWithItem {
		var itemPath string
		var isFile int
		dbErr = db.QueryRow("SELECT COALESCE(path, ''), isFile FROM libraryItems WHERE id = ?", itemID).Scan(&itemPath, &isFile)
		if dbErr == nil && itemPath != "" {
			folder := itemPath
			if isFile != 0 {
				folder = filepath.Dir(itemPath)
			}
			metadataPath = filepath.Join(folder, "metadata.json")
		}
	} else {
		itemDir := filepath.Join(MetadataPath, "items", itemID)
		_ = os.MkdirAll(itemDir, 0755)
		metadataPath = filepath.Join(itemDir, "metadata.json")
	}

	if metadataPath == "" || !utils.IsSafeFilePath(db, MetadataPath, metadataPath) {
		return nil
	}

	if mediaType == "book" {
		var title, subtitle, publisher, publishedYear, publishedDate, description, isbn, asin, language, narratorsRaw, tagsRaw, genresRaw string
		var explicitVal, abridgedVal int
		dbErr = db.QueryRow(`
			SELECT COALESCE(title, ''), COALESCE(subtitle, ''), COALESCE(publishedYear, ''), COALESCE(publishedDate, ''), COALESCE(publisher, ''), COALESCE(description, ''), COALESCE(isbn, ''), COALESCE(asin, ''), COALESCE(language, ''), explicit, abridged, COALESCE(narrators, '[]'), COALESCE(tags, '[]'), COALESCE(genres, '[]')
			FROM books WHERE id = ?
		`, mediaID).Scan(&title, &subtitle, &publishedYear, &publishedDate, &publisher, &description, &isbn, &asin, &language, &explicitVal, &abridgedVal, &narratorsRaw, &tagsRaw, &genresRaw)
		if dbErr != nil {
			return dbErr
		}

		var narrators, tags, genres []string
		_ = json.Unmarshal([]byte(narratorsRaw), &narrators)
		_ = json.Unmarshal([]byte(tagsRaw), &tags)
		_ = json.Unmarshal([]byte(genresRaw), &genres)

		var authors []string
		rows, authorErr := db.Query(`
			SELECT COALESCE(a.name, '') FROM authors a
			JOIN bookAuthors ba ON a.id = ba.authorId
			WHERE ba.bookId = ?
		`, mediaID)
		if authorErr == nil {
			defer rows.Close()
			for rows.Next() {
				var name string
				if errScan := rows.Scan(&name); errScan == nil {
					authors = append(authors, name)
				}
			}
		}

		metaData := map[string]interface{}{
			"title":         title,
			"subtitle":      subtitle,
			"authors":       authors,
			"narrators":     narrators,
			"publisher":     publisher,
			"publishedYear": publishedYear,
			"publishedDate": publishedDate,
			"description":   description,
			"isbn":          isbn,
			"asin":          asin,
			"language":      language,
			"explicit":      explicitVal != 0,
			"abridged":      abridgedVal != 0,
			"tags":          tags,
			"genres":        genres,
		}
		metaJSON, marshalErr := json.MarshalIndent(metaData, "", "  ")
		if marshalErr == nil {
			_ = os.WriteFile(metadataPath, metaJSON, 0644)
		}
	} else if mediaType == "podcast" {
		var title, author, description, language, tagsRaw, genresRaw string
		var explicitVal int
		dbErr = db.QueryRow(`
			SELECT COALESCE(title, ''), COALESCE(author, ''), COALESCE(description, ''), COALESCE(language, ''), explicit, COALESCE(tags, '[]'), COALESCE(genres, '[]')
			FROM podcasts WHERE id = ?
		`, mediaID).Scan(&title, &author, &description, &language, &explicitVal, &tagsRaw, &genresRaw)
		if dbErr != nil {
			return dbErr
		}

		var tags, genres []string
		_ = json.Unmarshal([]byte(tagsRaw), &tags)
		_ = json.Unmarshal([]byte(genresRaw), &genres)

		metaData := map[string]interface{}{
			"title":       title,
			"author":      author,
			"description": description,
			"language":    language,
			"explicit":    explicitVal != 0,
			"tags":        tags,
			"genres":      genres,
		}
		metaJSON, marshalErr := json.MarshalIndent(metaData, "", "  ")
		if marshalErr == nil {
			_ = os.WriteFile(metadataPath, metaJSON, 0644)
		}
	}

	return nil
}
