package scanner

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	log "audiobookshelf/internal/logger"
)

func getTableColumnsTx(tx *sql.Tx, tableName string) map[string]bool {
	columns := make(map[string]bool)
	rows, err := tx.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return columns
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltVal sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltVal, &pk); err != nil {
			log.Printf("[Scanner] Failed to scan table column info: %v", err)
			continue
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		log.Printf("[Scanner] Table info iteration error for table %s: %v", tableName, err)
	}
	return columns
}

// InsertAuthor inserts an author record into the authors table.
func InsertAuthor(tx *sql.Tx, id, name, lastFirst, libraryID string) error {
	return insertAuthor(tx, id, name, lastFirst, libraryID)
}

func insertAuthor(tx *sql.Tx, id, name, lastFirst, libraryID string) error {
	cols := getTableColumnsTx(tx, "authors")
	if len(cols) == 0 {
		return nil
	}

	var colNames []string
	var placeholders []string
	var args []interface{}

	addCol := func(name string, val interface{}) {
		if cols[name] {
			colNames = append(colNames, name)
			placeholders = append(placeholders, "?")
			args = append(args, val)
		}
	}

	addCol("id", id)
	addCol("name", name)
	addCol("lastFirst", lastFirst)
	addCol("libraryId", libraryID)

	nowStr := time.Now().Format("2006-01-02 15:04:05.000")
	addCol("createdAt", nowStr)
	addCol("updatedAt", nowStr)

	query := fmt.Sprintf("INSERT OR IGNORE INTO authors (%s) VALUES (%s)",
		strings.Join(colNames, ", "),
		strings.Join(placeholders, ", "))
	_, err := tx.Exec(query, args...)
	return err
}

// InsertBookAuthor inserts a bookAuthors association.
func InsertBookAuthor(tx *sql.Tx, bookID, authorID string) error {
	return insertBookAuthor(tx, bookID, authorID)
}

func insertBookAuthor(tx *sql.Tx, bookID, authorID string) error {
	cols := getTableColumnsTx(tx, "bookAuthors")
	if len(cols) == 0 {
		return nil
	}
	var colNames []string
	var placeholders []string
	var args []interface{}

	addCol := func(name string, val interface{}) {
		if cols[name] {
			colNames = append(colNames, name)
			placeholders = append(placeholders, "?")
			args = append(args, val)
		}
	}

	addCol("bookId", bookID)
	addCol("authorId", authorID)
	nowStr := time.Now().Format("2006-01-02 15:04:05.000")
	addCol("createdAt", nowStr)
	addCol("updatedAt", nowStr)

	query := fmt.Sprintf("INSERT OR IGNORE INTO bookAuthors (%s) VALUES (%s)",
		strings.Join(colNames, ", "),
		strings.Join(placeholders, ", "))
	_, err := tx.Exec(query, args...)
	return err
}

// InsertSeries inserts a series record into the series table.
func InsertSeries(tx *sql.Tx, id, name, libraryID string) error {
	return insertSeries(tx, id, name, libraryID)
}

func insertSeries(tx *sql.Tx, id, name, libraryID string) error {
	cols := getTableColumnsTx(tx, "series")
	if len(cols) == 0 {
		return nil
	}
	var colNames []string
	var placeholders []string
	var args []interface{}

	addCol := func(name string, val interface{}) {
		if cols[name] {
			colNames = append(colNames, name)
			placeholders = append(placeholders, "?")
			args = append(args, val)
		}
	}

	addCol("id", id)
	addCol("name", name)
	addCol("libraryId", libraryID)
	nowStr := time.Now().Format("2006-01-02 15:04:05.000")
	addCol("createdAt", nowStr)
	addCol("updatedAt", nowStr)

	query := fmt.Sprintf("INSERT OR IGNORE INTO series (%s) VALUES (%s)",
		strings.Join(colNames, ", "),
		strings.Join(placeholders, ", "))
	_, err := tx.Exec(query, args...)
	return err
}

// InsertBookSeries inserts a bookSeries association.
func InsertBookSeries(tx *sql.Tx, bookID, seriesID, sequence string) error {
	return insertBookSeries(tx, bookID, seriesID, sequence)
}

func insertBookSeries(tx *sql.Tx, bookID, seriesID, sequence string) error {
	cols := getTableColumnsTx(tx, "bookSeries")
	if len(cols) == 0 {
		return nil
	}
	var colNames []string
	var placeholders []string
	var args []interface{}

	addCol := func(name string, val interface{}) {
		if cols[name] {
			colNames = append(colNames, name)
			placeholders = append(placeholders, "?")
			args = append(args, val)
		}
	}

	addCol("bookId", bookID)
	addCol("seriesId", seriesID)
	addCol("sequence", sequence)
	nowStr := time.Now().Format("2006-01-02 15:04:05.000")
	addCol("createdAt", nowStr)
	addCol("updatedAt", nowStr)

	query := fmt.Sprintf("INSERT OR IGNORE INTO bookSeries (%s) VALUES (%s)",
		strings.Join(colNames, ", "),
		strings.Join(placeholders, ", "))
	_, err := tx.Exec(query, args...)
	return err
}

func tableExists(db *sql.DB, name string) bool {
	var count int
	err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", name).Scan(&count)
	return err == nil && count > 0
}

func tableExistsTx(tx *sql.Tx, name string) bool {
	var count int
	err := tx.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", name).Scan(&count)
	return err == nil && count > 0
}
