package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

// serveOPDSLibraryItems generates Atom feeds for the items in the library.
func serveOPDSLibraryItems(w http.ResponseWriter, r *http.Request, db *sql.DB, user *core.UserSession, lib *idb.LibraryJSON, sortByRecent bool) {
	pageVal := 0
	if r.URL.Query().Get("page") != "" {
		fmt.Sscanf(r.URL.Query().Get("page"), "%d", &pageVal)
	}

	limitVal := 20

	sortBy := "media.metadata.title"
	sortDesc := false
	if sortByRecent {
		sortBy = "addedAt"
		sortDesc = true
	}

	opts := idb.GetFilteredLibraryItemsOptions{
		LibraryID: lib.ID,
		User:      user,
		SortBy:    sortBy,
		SortDesc:  sortDesc,
		Limit:     limitVal,
		Page:      pageVal,
		MediaType: lib.MediaType,
		Minified:  false,
	}

	results, total, err := idb.GetFilteredLibraryItems(db, opts)
	if err != nil {
		log.Errorf("[OPDS] Failed to query items: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	updatedStr := time.Now().UTC().Format(time.RFC3339)
	actionStr := "all"
	if sortByRecent {
		actionStr = "recent"
	}

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	sb.WriteString(`<feed xmlns="http://www.w3.org/2005/Atom" xmlns:opds="http://opds-spec.org/2012/OPDS">` + "\n")
	sb.WriteString(fmt.Sprintf("  <id>urn:uuid:%s-%s-%d</id>\n", lib.ID, actionStr, pageVal))
	sb.WriteString(fmt.Sprintf("  <title>%s - %s</title>\n", html.EscapeString(lib.Name), actionStr))
	sb.WriteString(fmt.Sprintf("  <updated>%s</updated>\n", updatedStr))
	sb.WriteString("  <author><name>Audiobookshelf Go</name></author>\n")
	sb.WriteString(fmt.Sprintf("  <link rel=\"self\" href=\"/opds/v1.2/libraries/%s/%s?page=%d\" type=\"application/atom+xml;profile=opds-catalog;kind=acquisition\"/>\n", lib.ID, actionStr, pageVal))
	sb.WriteString("  <link rel=\"start\" href=\"/opds\" type=\"application/atom+xml;profile=opds-catalog;kind=navigation\"/>\n")

	// Pagination links
	if (pageVal+1)*limitVal < total {
		sb.WriteString(fmt.Sprintf("  <link rel=\"next\" href=\"/opds/v1.2/libraries/%s/%s?page=%d\" type=\"application/atom+xml;profile=opds-catalog;kind=acquisition\"/>\n", lib.ID, actionStr, pageVal+1))
	}
	if pageVal > 0 {
		sb.WriteString(fmt.Sprintf("  <link rel=\"previous\" href=\"/opds/v1.2/libraries/%s/%s?page=%d\" type=\"application/atom+xml;profile=opds-catalog;kind=acquisition\"/>\n", lib.ID, actionStr, pageVal-1))
	}

	writeItemEntries(&sb, results, r)

	sb.WriteString("</feed>")

	w.Header().Set("Content-Type", "application/atom+xml;profile=opds-catalog;charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(sb.String()))
}

// serveOPDSSearch searches items in the library and formats the response as an Atom feed.
func serveOPDSSearch(w http.ResponseWriter, r *http.Request, db *sql.DB, user *core.UserSession, lib *idb.LibraryJSON) {
	q := r.URL.Query().Get("q")
	if q == "" {
		q = r.URL.Query().Get("query")
	}

	opts := idb.GetFilteredLibraryItemsOptions{
		LibraryID: lib.ID,
		User:      user,
		SortBy:    "media.metadata.title",
		MediaType: lib.MediaType,
		Minified:  false,
		Search:    q,
		Limit:     50,
	}

	results, _, err := idb.GetFilteredLibraryItems(db, opts)
	if err != nil {
		log.Errorf("[OPDS] Failed to query items for search: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	updatedStr := time.Now().UTC().Format(time.RFC3339)
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	sb.WriteString(`<feed xmlns="http://www.w3.org/2005/Atom" xmlns:opds="http://opds-spec.org/2012/OPDS">` + "\n")
	sb.WriteString(fmt.Sprintf("  <id>urn:uuid:%s-search-%s</id>\n", lib.ID, html.EscapeString(q)))
	sb.WriteString(fmt.Sprintf("  <title>Search Results: %s</title>\n", html.EscapeString(q)))
	sb.WriteString(fmt.Sprintf("  <updated>%s</updated>\n", updatedStr))
	sb.WriteString("  <author><name>Audiobookshelf Go</name></author>\n")
	sb.WriteString(fmt.Sprintf("  <link rel=\"self\" href=\"/opds/v1.2/libraries/%s/search?q=%s\" type=\"application/atom+xml;profile=opds-catalog;kind=acquisition\"/>\n", lib.ID, url.QueryEscape(q)))
	sb.WriteString("  <link rel=\"start\" href=\"/opds\" type=\"application/atom+xml;profile=opds-catalog;kind=navigation\"/>\n")

	writeItemEntries(&sb, results, r)

	sb.WriteString("</feed>")

	w.Header().Set("Content-Type", "application/atom+xml;profile=opds-catalog;charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(sb.String()))
}
