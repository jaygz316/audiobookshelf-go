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

// serveOPDSRoot generates the root OPDS XML catalog of libraries.
func serveOPDSRoot(w http.ResponseWriter, r *http.Request, db *sql.DB, user *core.UserSession) {
	libs, err := idb.GetLibraries(db)
	if err != nil {
		log.Errorf("[OPDS] Failed to get libraries: %v", err)
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

	// Subsection: Authors
	sb.WriteString("  <entry>\n")
	sb.WriteString("    <title>Authors</title>\n")
	sb.WriteString(fmt.Sprintf("    <id>urn:uuid:%s-authors</id>\n", lib.ID))
	sb.WriteString(fmt.Sprintf("    <updated>%s</updated>\n", updatedStr))
	sb.WriteString("    <content type=\"text\">Browse items by author</content>\n")
	sb.WriteString(fmt.Sprintf("    <link rel=\"subsection\" href=\"/opds/v1.2/libraries/%s/authors\" type=\"application/atom+xml;profile=opds-catalog;kind=navigation\"/>\n", lib.ID))
	sb.WriteString("  </entry>\n")

	// Subsection: Series
	sb.WriteString("  <entry>\n")
	sb.WriteString("    <title>Series</title>\n")
	sb.WriteString(fmt.Sprintf("    <id>urn:uuid:%s-series</id>\n", lib.ID))
	sb.WriteString(fmt.Sprintf("    <updated>%s</updated>\n", updatedStr))
	sb.WriteString("    <content type=\"text\">Browse items by series</content>\n")
	sb.WriteString(fmt.Sprintf("    <link rel=\"subsection\" href=\"/opds/v1.2/libraries/%s/series\" type=\"application/atom+xml;profile=opds-catalog;kind=navigation\"/>\n", lib.ID))
	sb.WriteString("  </entry>\n")

	// Subsection: Collections
	sb.WriteString("  <entry>\n")
	sb.WriteString("    <title>Collections</title>\n")
	sb.WriteString(fmt.Sprintf("    <id>urn:uuid:%s-collections</id>\n", lib.ID))
	sb.WriteString(fmt.Sprintf("    <updated>%s</updated>\n", updatedStr))
	sb.WriteString("    <content type=\"text\">Browse collections</content>\n")
	sb.WriteString(fmt.Sprintf("    <link rel=\"subsection\" href=\"/opds/v1.2/libraries/%s/collections\" type=\"application/atom+xml;profile=opds-catalog;kind=navigation\"/>\n", lib.ID))
	sb.WriteString("  </entry>\n")

	// Subsection: Playlists
	sb.WriteString("  <entry>\n")
	sb.WriteString("    <title>Playlists</title>\n")
	sb.WriteString(fmt.Sprintf("    <id>urn:uuid:%s-playlists</id>\n", lib.ID))
	sb.WriteString(fmt.Sprintf("    <updated>%s</updated>\n", updatedStr))
	sb.WriteString("    <content type=\"text\">Browse your playlists</content>\n")
	sb.WriteString(fmt.Sprintf("    <link rel=\"subsection\" href=\"/opds/v1.2/libraries/%s/playlists\" type=\"application/atom+xml;profile=opds-catalog;kind=navigation\"/>\n", lib.ID))
	sb.WriteString("  </entry>\n")

	// Subsection: Items
	// Note: We don't have direct library-level items feed sub-navigation.

	sb.WriteString("</feed>")

	w.Header().Set("Content-Type", "application/atom+xml;profile=opds-catalog;charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(sb.String()))
}
