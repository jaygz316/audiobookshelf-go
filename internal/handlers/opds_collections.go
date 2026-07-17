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

// serveOPDSCollections lists all collections in the library.
func serveOPDSCollections(w http.ResponseWriter, r *http.Request, db *sql.DB, user *core.UserSession, lib *idb.LibraryJSON) {
	rows, err := db.Query(`
		SELECT id, name, description
		FROM collections
		WHERE libraryId = ?
		ORDER BY name ASC
	`, lib.ID)
	if err != nil {
		log.Errorf("[OPDS] Failed to query collections: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	updatedStr := time.Now().UTC().Format(time.RFC3339)
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	sb.WriteString(`<feed xmlns="http://www.w3.org/2005/Atom" xmlns:opds="http://opds-spec.org/2012/OPDS">` + "\n")
	sb.WriteString(fmt.Sprintf("  <id>urn:uuid:%s-collections</id>\n", lib.ID))
	sb.WriteString(fmt.Sprintf("  <title>%s - Collections</title>\n", html.EscapeString(lib.Name)))
	sb.WriteString(fmt.Sprintf("  <updated>%s</updated>\n", updatedStr))
	sb.WriteString("  <author><name>Audiobookshelf Go</name></author>\n")
	sb.WriteString(fmt.Sprintf("  <link rel=\"self\" href=\"/opds/v1.2/libraries/%s/collections\" type=\"application/atom+xml;profile=opds-catalog;kind=navigation\"/>\n", lib.ID))
	sb.WriteString("  <link rel=\"start\" href=\"/opds\" type=\"application/atom+xml;profile=opds-catalog;kind=navigation\"/>\n")

	for rows.Next() {
		var id, name string
		var description sql.NullString
		if err := rows.Scan(&id, &name, &description); err == nil {
			desc := "Browse items in collection: " + name
			if description.Valid && description.String != "" {
				desc = description.String
			}
			sb.WriteString("  <entry>\n")
			sb.WriteString(fmt.Sprintf("    <title>%s</title>\n", html.EscapeString(name)))
			sb.WriteString(fmt.Sprintf("    <id>urn:uuid:%s-collections-%s</id>\n", lib.ID, id))
			sb.WriteString(fmt.Sprintf("    <updated>%s</updated>\n", updatedStr))
			sb.WriteString(fmt.Sprintf("    <content type=\"text\">%s</content>\n", html.EscapeString(desc)))
			sb.WriteString(fmt.Sprintf("    <link rel=\"subsection\" href=\"/opds/v1.2/libraries/%s/collections/%s\" type=\"application/atom+xml;profile=opds-catalog;kind=acquisition\"/>\n", lib.ID, id))
			sb.WriteString("  </entry>\n")
		}
	}

	sb.WriteString("</feed>")
	w.Header().Set("Content-Type", "application/atom+xml;profile=opds-catalog;charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(sb.String()))
}

// serveOPDSCollectionItems lists all items in the specified collection.
func serveOPDSCollectionItems(w http.ResponseWriter, r *http.Request, db *sql.DB, user *core.UserSession, lib *idb.LibraryJSON, collectionID string) {
	var collectionName string
	err := db.QueryRow("SELECT name FROM collections WHERE id = ?", collectionID).Scan(&collectionName)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Collection not found", http.StatusNotFound)
		} else {
			log.Errorf("[OPDS] Failed to query collection name: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	rows, err := db.Query(`
		SELECT li.id
		FROM libraryItems li
		JOIN collectionBooks cb ON li.mediaId = cb.bookId AND li.mediaType = 'book'
		WHERE li.libraryId = ? AND cb.collectionId = ?
		ORDER BY cb."order" ASC
	`, lib.ID, collectionID)
	if err != nil {
		log.Errorf("[OPDS] Failed to query collection items: %v", err)
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
	sb.WriteString(fmt.Sprintf("  <id>urn:uuid:%s-collections-%s-items</id>\n", lib.ID, collectionID))
	sb.WriteString(fmt.Sprintf("  <title>Collection: %s</title>\n", html.EscapeString(collectionName)))
	sb.WriteString(fmt.Sprintf("  <updated>%s</updated>\n", updatedStr))
	sb.WriteString("  <author><name>Audiobookshelf Go</name></author>\n")
	sb.WriteString(fmt.Sprintf("  <link rel=\"self\" href=\"/opds/v1.2/libraries/%s/collections/%s\" type=\"application/atom+xml;profile=opds-catalog;kind=acquisition\"/>\n", lib.ID, collectionID))
	sb.WriteString("  <link rel=\"start\" href=\"/opds\" type=\"application/atom+xml;profile=opds-catalog;kind=navigation\"/>\n")

	writeItemEntries(&sb, items, r)

	sb.WriteString("</feed>")
	w.Header().Set("Content-Type", "application/atom+xml;profile=opds-catalog;charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(sb.String()))
}
