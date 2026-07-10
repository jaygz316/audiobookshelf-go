package handlers

import (
	"database/sql"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

// ServeOPDS handles all OPDS feed endpoints.
func ServeOPDS(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		path := r.URL.Path
		// Strip trailing slash if present (except for root path)
		if len(path) > 1 && strings.HasSuffix(path, "/") {
			path = strings.TrimSuffix(path, "/")
		}

		// Root /opds or /opds/v1.2/catalog
		if path == "/opds" || path == "/opds/v1.2/catalog" {
			serveOPDSRoot(w, r, db, user)
			return
		}

		// /opds/v1.2/libraries/{libraryID}
		// /opds/v1.2/libraries/{libraryID}/all
		// /opds/v1.2/libraries/{libraryID}/recent
		// /opds/v1.2/libraries/{libraryID}/search
		parts := strings.Split(strings.TrimPrefix(path, "/opds/"), "/")
		if len(parts) >= 3 && parts[0] == "v1.2" && parts[1] == "libraries" {
			libraryID := parts[2]
			if !user.CanAccessLibrary(libraryID) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			lib, err := idb.GetLibraryByID(db, libraryID)
			if err != nil || lib == nil {
				http.Error(w, "Library not found", http.StatusNotFound)
				return
			}

			subAction := ""
			if len(parts) >= 4 {
				subAction = parts[3]
			}

			switch subAction {
			case "all":
				serveOPDSLibraryItems(w, r, db, user, lib, false)
			case "recent":
				serveOPDSLibraryItems(w, r, db, user, lib, true)
			case "search":
				serveOPDSSearch(w, r, db, user, lib)
			case "":
				serveOPDSLibraryDetails(w, r, db, user, lib)
			default:
				http.NotFound(w, r)
			}
			return
		}

		http.NotFound(w, r)
	}
}

// serveOPDSRoot generates the root OPDS XML catalog of libraries.
func serveOPDSRoot(w http.ResponseWriter, r *http.Request, db *sql.DB, user *core.UserSession) {
	libs, err := idb.GetLibraries(db)
	if err != nil {
		log.Printf("[OPDS] Failed to get libraries: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Filter accessible libraries
	var accessibleLibs []*idb.LibraryJSON
	for _, lib := range libs {
		if user.CanAccessLibrary(lib.ID) {
			accessibleLibs = append(accessibleLibs, lib)
		}
	}

	updatedStr := time.Now().UTC().Format(time.RFC3339)
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	sb.WriteString(`<feed xmlns="http://www.w3.org/2005/Atom" xmlns:opds="http://opds-spec.org/2012/OPDS">` + "\n")
	sb.WriteString(`  <id>urn:uuid:audiobookshelf-go-root</id>` + "\n")
	sb.WriteString(`  <title>Audiobookshelf Go Catalog</title>` + "\n")
	sb.WriteString(fmt.Sprintf(`  <updated>%s</updated>`, updatedStr) + "\n")
	sb.WriteString(`  <author><name>Audiobookshelf Go</name></author>` + "\n")
	sb.WriteString(`  <link rel="self" href="/opds" type="application/atom+xml;profile=opds-catalog;kind=navigation"/>` + "\n")
	sb.WriteString(`  <link rel="start" href="/opds" type="application/atom+xml;profile=opds-catalog;kind=navigation"/>` + "\n")

	for _, lib := range accessibleLibs {
		libName := html.EscapeString(lib.Name)
		libUpdated := time.Unix(0, lib.LastUpdate*int64(time.Millisecond)).UTC().Format(time.RFC3339)
		sb.WriteString(`  <entry>` + "\n")
		sb.WriteString(fmt.Sprintf(`    <title>%s</title>`, libName) + "\n")
		sb.WriteString(fmt.Sprintf(`    <id>urn:uuid:%s</id>`, lib.ID) + "\n")
		sb.WriteString(fmt.Sprintf(`    <updated>%s</updated>`, libUpdated) + "\n")
		sb.WriteString(fmt.Sprintf(`    <content type="text">Browse library: %s</content>`, libName) + "\n")
		sb.WriteString(fmt.Sprintf(`    <link rel="subsection" href="/opds/v1.2/libraries/%s" type="application/atom+xml;profile=opds-catalog;kind=navigation"/>`, lib.ID) + "\n")
		sb.WriteString(`  </entry>` + "\n")
	}

	sb.WriteString(`</feed>`)

	w.Header().Set("Content-Type", "application/atom+xml;profile=opds-catalog;charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(sb.String()))
}

// serveOPDSLibraryDetails serves navigation links and recent items inside a library.
func serveOPDSLibraryDetails(w http.ResponseWriter, r *http.Request, db *sql.DB, user *core.UserSession, lib *idb.LibraryJSON) {
	updatedStr := time.Now().UTC().Format(time.RFC3339)
	libName := html.EscapeString(lib.Name)

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	sb.WriteString(`<feed xmlns="http://www.w3.org/2005/Atom" xmlns:opds="http://opds-spec.org/2012/OPDS">` + "\n")
	sb.WriteString(fmt.Sprintf("  <id>urn:uuid:%s</id>", lib.ID) + "\n")
	sb.WriteString(fmt.Sprintf("  <title>%s</title>", libName) + "\n")
	sb.WriteString(fmt.Sprintf("  <updated>%s</updated>", updatedStr) + "\n")
	sb.WriteString("  <author><name>Audiobookshelf Go</name></author>\n")
	sb.WriteString(fmt.Sprintf("  <link rel=\"self\" href=\"/opds/v1.2/libraries/%s\" type=\"application/atom+xml;profile=opds-catalog;kind=navigation\"/>\n", lib.ID))
	sb.WriteString("  <link rel=\"start\" href=\"/opds\" type=\"application/atom+xml;profile=opds-catalog;kind=navigation\"/>\n")

	// Search Link Template
	sb.WriteString(fmt.Sprintf("  <link rel=\"search\" href=\"/opds/v1.2/libraries/%s/search?q={searchTerms}\" type=\"application/atom+xml\"/>\n", lib.ID))

	// Subsection: All Items
	sb.WriteString("  <entry>\n")
	sb.WriteString("    <title>All Items</title>\n")
	sb.WriteString(fmt.Sprintf("    <id>urn:uuid:%s-all</id>\n", lib.ID))
	sb.WriteString(fmt.Sprintf("    <updated>%s</updated>\n", updatedStr))
	sb.WriteString("    <content type=\"text\">Show all items in this library</content>\n")
	sb.WriteString(fmt.Sprintf("    <link rel=\"subsection\" href=\"/opds/v1.2/libraries/%s/all\" type=\"application/atom+xml;profile=opds-catalog;kind=acquisition\"/>\n", lib.ID))
	sb.WriteString("  </entry>\n")

	// Subsection: Recent
	sb.WriteString("  <entry>\n")
	sb.WriteString("    <title>Recent</title>\n")
	sb.WriteString(fmt.Sprintf("    <id>urn:uuid:%s-recent</id>\n", lib.ID))
	sb.WriteString(fmt.Sprintf("    <updated>%s</updated>\n", updatedStr))
	sb.WriteString("    <content type=\"text\">Show recently added items</content>\n")
	sb.WriteString(fmt.Sprintf("    <link rel=\"subsection\" href=\"/opds/v1.2/libraries/%s/recent\" type=\"application/atom+xml;profile=opds-catalog;kind=acquisition\"/>\n", lib.ID))
	sb.WriteString("  </entry>\n")

	sb.WriteString("</feed>")

	w.Header().Set("Content-Type", "application/atom+xml;profile=opds-catalog;charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(sb.String()))
}

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
		log.Printf("[OPDS] Failed to query items: %v", err)
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
		log.Printf("[OPDS] Failed to query items for search: %v", err)
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

// writeItemEntries formats each minified library item as an Atom XML entry.
func writeItemEntries(sb *strings.Builder, items []*idb.LibraryItemMinifiedJSON, r *http.Request) {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if xfh := r.Header.Get("X-Forwarded-Host"); xfh != "" {
		host = xfh
	}

	baseURL := fmt.Sprintf("%s://%s", scheme, host)

	for _, item := range items {
		title := ""
		author := ""
		narrator := ""
		description := ""
		mimeType := "application/zip" // Default for audiobook download folders

		if item.MediaType == "book" {
			if book, ok := item.Media.(*idb.BookMinifiedJSON); ok && book != nil {
				title = book.Metadata.Title
				if book.Metadata.Subtitle != nil {
					title = fmt.Sprintf("%s - %s", title, *book.Metadata.Subtitle)
				}
				author = book.Metadata.AuthorName
				narrator = book.Metadata.NarratorName
				if book.Metadata.Description != nil {
					description = *book.Metadata.Description
				}
				if book.EbookFormat != nil && *book.EbookFormat != "" {
					fmtStr := strings.ToLower(*book.EbookFormat)
					if fmtStr == "epub" {
						mimeType = "application/epub+zip"
					} else if fmtStr == "pdf" {
						mimeType = "application/pdf"
					} else if fmtStr == "mobi" {
						mimeType = "application/x-mobipocket-ebook"
					}
				}
			}
		} else if item.MediaType == "podcast" {
			if podcast, ok := item.Media.(*idb.PodcastMinifiedJSON); ok && podcast != nil {
				title = podcast.Metadata.Title
				if podcast.Metadata.Author != nil {
					author = *podcast.Metadata.Author
				}
				if podcast.Metadata.Description != nil {
					description = *podcast.Metadata.Description
				}
			}
		}

		updatedTime := time.Unix(0, item.UpdatedAt*int64(time.Millisecond)).UTC().Format(time.RFC3339)

		sb.WriteString("  <entry>\n")
		sb.WriteString(fmt.Sprintf("    <title>%s</title>\n", html.EscapeString(title)))
		sb.WriteString(fmt.Sprintf("    <id>urn:uuid:%s</id>\n", item.ID))
		sb.WriteString(fmt.Sprintf("    <updated>%s</updated>\n", updatedTime))
		
		if author != "" {
			sb.WriteString(fmt.Sprintf("    <author><name>%s</name></author>\n", html.EscapeString(author)))
		}
		if narrator != "" {
			sb.WriteString(fmt.Sprintf("    <contributor role=\"nrt\"><name>%s</name></contributor>\n", html.EscapeString(narrator)))
		}
		
		if description != "" {
			sb.WriteString(fmt.Sprintf("    <content type=\"text\">%s</content>\n", html.EscapeString(description)))
		} else {
			sb.WriteString("    <content type=\"text\">No description available.</content>\n")
		}

		// Covers
		sb.WriteString(fmt.Sprintf("    <link rel=\"http://opds-spec.org/image\" href=\"%s/api/items/%s/cover\" type=\"image/jpeg\"/>\n", baseURL, item.ID))
		sb.WriteString(fmt.Sprintf("    <link rel=\"http://opds-spec.org/image/thumbnail\" href=\"%s/api/items/%s/cover?width=200\" type=\"image/jpeg\"/>\n", baseURL, item.ID))

		// Acquisition/Download Link
		sb.WriteString(fmt.Sprintf("    <link rel=\"http://opds-spec.org/acquisition\" href=\"%s/api/items/%s/download\" type=\"%s\"/>\n", baseURL, item.ID, mimeType))

		sb.WriteString("  </entry>\n")
	}
}
