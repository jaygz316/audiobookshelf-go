package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

// serveOPDSAuthors lists all authors in the library as navigation entries.
func serveOPDSAuthors(w http.ResponseWriter, r *http.Request, db *sql.DB, user *core.UserSession, lib *idb.LibraryJSON) {
	rows, err := db.Query(`
		SELECT id, name, description
		FROM authors
		WHERE libraryId = ?
		ORDER BY name ASC
	`, lib.ID)
	if err != nil {
		log.Errorf("[OPDS] Failed to query authors: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	updatedStr := time.Now().UTC().Format(time.RFC3339)
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	sb.WriteString(`<feed xmlns="http://www.w3.org/2005/Atom" xmlns:opds="http://opds-spec.org/2012/OPDS">` + "\n")
	sb.WriteString(fmt.Sprintf("  <id>urn:uuid:%s-authors</id>\n", lib.ID))
	sb.WriteString(fmt.Sprintf("  <title>%s - Authors</title>\n", html.EscapeString(lib.Name)))
	sb.WriteString(fmt.Sprintf("  <updated>%s</updated>\n", updatedStr))
	sb.WriteString("  <author><name>Audiobookshelf Go</name></author>\n")
	sb.WriteString(fmt.Sprintf("  <link rel=\"self\" href=\"/opds/v1.2/libraries/%s/authors\" type=\"application/atom+xml;profile=opds-catalog;kind=navigation\"/>\n", lib.ID))
	sb.WriteString("  <link rel=\"start\" href=\"/opds\" type=\"application/atom+xml;profile=opds-catalog;kind=navigation\"/>\n")

	for rows.Next() {
		var id, name string
		var description sql.NullString
		if err := rows.Scan(&id, &name, &description); err == nil {
			desc := "Browse items by " + name
			if description.Valid && description.String != "" {
				desc = description.String
			}
			sb.WriteString("  <entry>\n")
			sb.WriteString(fmt.Sprintf("    <title>%s</title>\n", html.EscapeString(name)))
			sb.WriteString(fmt.Sprintf("    <id>urn:uuid:%s-authors-%s</id>\n", lib.ID, id))
			sb.WriteString(fmt.Sprintf("    <updated>%s</updated>\n", updatedStr))
			sb.WriteString(fmt.Sprintf("    <content type=\"text\">%s</content>\n", html.EscapeString(desc)))
			sb.WriteString(fmt.Sprintf("    <link rel=\"subsection\" href=\"/opds/v1.2/libraries/%s/authors/%s\" type=\"application/atom+xml;profile=opds-catalog;kind=acquisition\"/>\n", lib.ID, id))
			sb.WriteString("  </entry>\n")
		}
	}

	sb.WriteString("</feed>")
	w.Header().Set("Content-Type", "application/atom+xml;profile=opds-catalog;charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(sb.String()))
}

// serveOPDSAuthorItems lists all items matching the specified author.
func serveOPDSAuthorItems(w http.ResponseWriter, r *http.Request, db *sql.DB, user *core.UserSession, lib *idb.LibraryJSON, authorID string) {
	var authorName string
	err := db.QueryRow("SELECT name FROM authors WHERE id = ?", authorID).Scan(&authorName)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Author not found", http.StatusNotFound)
		} else {
			log.Errorf("[OPDS] Failed to query author name: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	rows, err := db.Query(`
		SELECT DISTINCT li.id
		FROM libraryItems li
		JOIN bookAuthors ba ON li.mediaId = ba.bookId AND li.mediaType = 'book'
		WHERE li.libraryId = ? AND ba.authorId = ?
		ORDER BY li.createdAt DESC
	`, lib.ID, authorID)
	if err != nil {
		log.Errorf("[OPDS] Failed to query author items: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var itemIDs []string
	for rows.Next() {
		var itemID string
		if err := rows.Scan(&itemID); err == nil {
			itemIDs = append(itemIDs, itemID)
		}
	}
	rows.Close()

	var items []*idb.LibraryItemMinifiedJSON
	for _, itemID := range itemIDs {
		if item, err := idb.GetLibraryItemMinifiedByID(db, itemID); err == nil && item != nil {
			if canUserAccessItemMinified(user, item) {
				items = append(items, item)
			}
		}
	}

	updatedStr := time.Now().UTC().Format(time.RFC3339)
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	sb.WriteString(`<feed xmlns="http://www.w3.org/2005/Atom" xmlns:opds="http://opds-spec.org/2012/OPDS">` + "\n")
	sb.WriteString(fmt.Sprintf("  <id>urn:uuid:%s-authors-%s-items</id>\n", lib.ID, authorID))
	sb.WriteString(fmt.Sprintf("  <title>Books by %s</title>\n", html.EscapeString(authorName)))
	sb.WriteString(fmt.Sprintf("  <updated>%s</updated>\n", updatedStr))
	sb.WriteString("  <author><name>Audiobookshelf Go</name></author>\n")
	sb.WriteString(fmt.Sprintf("  <link rel=\"self\" href=\"/opds/v1.2/libraries/%s/authors/%s\" type=\"application/atom+xml;profile=opds-catalog;kind=acquisition\"/>\n", lib.ID, authorID))
	sb.WriteString("  <link rel=\"start\" href=\"/opds\" type=\"application/atom+xml;profile=opds-catalog;kind=navigation\"/>\n")

	writeItemEntries(&sb, items, r)

	sb.WriteString("</feed>")
	w.Header().Set("Content-Type", "application/atom+xml;profile=opds-catalog;charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(sb.String()))
}
