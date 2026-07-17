package handlers

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/utils"
)

func updateAuthorMatchInDB(db *sql.DB, authorID string, name string, asin string, description string, localImagePath string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	nowStr := time.Now().Format("2006-01-02 15:04:05.000")
	lastFirst := utils.NameToLastFirst(name)

	if localImagePath != "" {
		_, err = tx.Exec(`
			UPDATE authors
			SET name = ?, lastFirst = ?, asin = ?, description = ?, imagePath = ?, updatedAt = ?
			WHERE id = ?
		`, name, lastFirst, asin, description, localImagePath, nowStr, authorID)
	} else {
		_, err = tx.Exec(`
			UPDATE authors
			SET name = ?, lastFirst = ?, asin = ?, description = ?, updatedAt = ?
			WHERE id = ?
		`, name, lastFirst, asin, description, nowStr, authorID)
	}

	if err != nil {
		return fmt.Errorf("failed to update author: %w", err)
	}

	// Update linked libraryItems caches
	rows, err := tx.Query("SELECT bookId FROM bookAuthors WHERE authorId = ?", authorID)
	if err == nil {
		var bookIDs []string
		for rows.Next() {
			var bid string
			if err := rows.Scan(&bid); err == nil {
				bookIDs = append(bookIDs, bid)
			}
		}
		rows.Close()

		for _, bid := range bookIDs {
			var authorNames []string
			var authorLastFirsts []string

			arows, err := tx.Query(`
				SELECT a.name, a.lastFirst
				FROM authors a
				JOIN bookAuthors ba ON ba.authorId = a.id
				WHERE ba.bookId = ?
			`, bid)
			if err == nil {
				for arows.Next() {
					var aName, aLastFirst string
					if err := arows.Scan(&aName, &aLastFirst); err == nil {
						authorNames = append(authorNames, aName)
						authorLastFirsts = append(authorLastFirsts, aLastFirst)
					}
				}
				arows.Close()
			}

			authorNamesStr := strings.Join(authorNames, ", ")
			authorLastFirstsStr := strings.Join(authorLastFirsts, ", ")

			_, _ = tx.Exec(`
				UPDATE libraryItems
				SET authorNamesFirstLast = ?, authorNamesLastFirst = ?, updatedAt = ?
				WHERE mediaId = ? AND mediaType = 'book'
			`, authorNamesStr, authorLastFirstsStr, nowStr, bid)
		}
	}

	return tx.Commit()
}

func updateAuthorBooksCacheAndMetadata(db *sql.DB, authorID string, name string, lastFirst string, asin string, description string) ([]BookUpdate, error) {
	srvSettings, srvErr := idb.GetServerSettings(db)

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	nowStr := time.Now().Format("2006-01-02 15:04:05.000")

	// Update the author table
	_, err = tx.Exec(`
		UPDATE authors
		SET name = ?, lastFirst = ?, asin = ?, description = ?, updatedAt = ?
		WHERE id = ?
	`, name, lastFirst, asin, description, nowStr, authorID)
	if err != nil {
		return nil, fmt.Errorf("failed to update author: %w", err)
	}

	var booksToUpdate []BookUpdate

	// Fetch all books linked to this author in bookAuthors
	rows, err := tx.Query("SELECT bookId FROM bookAuthors WHERE authorId = ?", authorID)
	if err == nil {
		var bookIDs []string
		for rows.Next() {
			var bid string
			if err := rows.Scan(&bid); err == nil {
				bookIDs = append(bookIDs, bid)
			}
		}
		rows.Close()

		// For each book, fetch all authors, join them, and update libraryItems table
		for _, bid := range bookIDs {
			var authorNames []string
			var authorLastFirsts []string

			arows, err := tx.Query(`
				SELECT a.name, a.lastFirst
				FROM authors a
				JOIN bookAuthors ba ON ba.authorId = a.id
				WHERE ba.bookId = ?
			`, bid)
			if err == nil {
				for arows.Next() {
					var aName, aLastFirst string
					if err := arows.Scan(&aName, &aLastFirst); err == nil {
						authorNames = append(authorNames, aName)
						authorLastFirsts = append(authorLastFirsts, aLastFirst)
					}
				}
				arows.Close()
			}

			authorNamesStr := strings.Join(authorNames, ", ")
			authorLastFirstsStr := strings.Join(authorLastFirsts, ", ")

			_, _ = tx.Exec(`
				UPDATE libraryItems
				SET authorNamesFirstLast = ?, authorNamesLastFirst = ?, updatedAt = ?
				WHERE mediaId = ? AND mediaType = 'book'
			`, authorNamesStr, authorLastFirstsStr, nowStr, bid)

			var itemID string
			_ = tx.QueryRow("SELECT id FROM libraryItems WHERE mediaId = ? AND mediaType = 'book'", bid).Scan(&itemID)

			var metadataPath string
			if srvErr == nil && srvSettings != nil && srvSettings.MetadataMarkdownWithItem {
				var itemPath string
				var isFile int
				dbErr := tx.QueryRow("SELECT path, isFile FROM libraryItems WHERE mediaId = ? AND mediaType = 'book'", bid).Scan(&itemPath, &isFile)
				if dbErr == nil && itemPath != "" {
					folder := itemPath
					if isFile != 0 {
						folder = filepath.Dir(itemPath)
					}
					metadataPath = filepath.Join(folder, "metadata.json")
				}
			} else if itemID != "" {
				metadataPath = filepath.Join(MetadataPath, "items", itemID, "metadata.json")
			}

			booksToUpdate = append(booksToUpdate, BookUpdate{
				bid:          bid,
				itemID:       itemID,
				authorNames:  authorNames,
				metadataPath: metadataPath,
			})
		}
	}

	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	return booksToUpdate, nil
}
