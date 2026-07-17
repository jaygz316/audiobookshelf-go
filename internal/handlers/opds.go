package handlers

import (
	"database/sql"
	"net/http"
	"strings"

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
			case "authors":
				if len(parts) >= 5 {
					serveOPDSAuthorItems(w, r, db, user, lib, parts[4])
				} else {
					serveOPDSAuthors(w, r, db, user, lib)
				}
			case "series":
				if len(parts) >= 5 {
					serveOPDSSeriesItems(w, r, db, user, lib, parts[4])
				} else {
					serveOPDSSeries(w, r, db, user, lib)
				}
			case "collections":
				if len(parts) >= 5 {
					serveOPDSCollectionItems(w, r, db, user, lib, parts[4])
				} else {
					serveOPDSCollections(w, r, db, user, lib)
				}
			case "playlists":
				if len(parts) >= 5 {
					serveOPDSPlaylistItems(w, r, db, user, lib, parts[4])
				} else {
					serveOPDSPlaylists(w, r, db, user, lib)
				}
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
