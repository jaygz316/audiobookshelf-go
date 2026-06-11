package main

// scanner.go — thin wrapper re-exporting scanner functions from internal/scanner.
// Note: LibraryItemMinifiedJSON, BookMinifiedJSON, PodcastMinifiedJSON are defined in db.go.
// Note: GetLibraryItemMinifiedByID is also defined in db.go (not wrapped here to avoid type conflicts).

import (
	iscanner "audiobookshelf/internal/scanner"
	"database/sql"
	"net/http"
)

// FileItem is an alias for the internal scanner FileItem type.
type FileItem = iscanner.FileItem

// ScanLibrary triggers a full scan of the given library. Matches watcher.ScanFunc signature.
func ScanLibrary(db *sql.DB, libraryID string) error {
	return iscanner.ScanLibrary(db, libraryID, SocketAuth)
}

// IsMediaFile checks if the given file extension is a supported media type.
func IsMediaFile(mediaType, ext string, audiobooksOnly bool) bool {
	return iscanner.IsMediaFile(mediaType, ext, audiobooksOnly)
}

// EmitLibraryItemEvent emits a library item event to all clients via the socket authority.
func EmitLibraryItemEvent(evt string, item *LibraryItemMinifiedJSON) {
	if item == nil {
		return
	}
	if SocketAuth != nil {
		SocketAuth.BroadcastToAll(evt, item)
	}
}

// EmitLibraryItemsEvent emits a library items event to all clients via the socket authority.
func EmitLibraryItemsEvent(evt string, item *LibraryItemMinifiedJSON) {
	if item == nil {
		return
	}
	if SocketAuth != nil {
		SocketAuth.BroadcastToAll(evt, item)
	}
}

// handleScanLibrary returns an HTTP handler for triggering a library scan.
func handleScanLibrary(db *sql.DB, libraryID string) http.HandlerFunc {
	return iscanner.HandleScanLibrary(db, libraryID, SocketAuth)
}

// nameToLastFirst converts "First Last" to "Last, First".
func nameToLastFirst(name string) string {
	return iscanner.NameToLastFirst(name)
}

// uuidStr returns a new UUID string.
func uuidStr() string {
	return iscanner.UUIDStr()
}

// insertAuthor inserts an author record.
func insertAuthor(tx *sql.Tx, id, name, lastFirst, libraryID string) error {
	return iscanner.InsertAuthor(tx, id, name, lastFirst, libraryID)
}

// insertBookAuthor inserts a book-author association.
func insertBookAuthor(tx *sql.Tx, bookID, authorID string) error {
	return iscanner.InsertBookAuthor(tx, bookID, authorID)
}

// insertSeries inserts a series record.
func insertSeries(tx *sql.Tx, id, name, libraryID string) error {
	return iscanner.InsertSeries(tx, id, name, libraryID)
}

// insertBookSeries inserts a book-series association.
func insertBookSeries(tx *sql.Tx, bookID, seriesID, sequence string) error {
	return iscanner.InsertBookSeries(tx, bookID, seriesID, sequence)
}

// FilenameMetadata is an alias for the internal scanner FilenameMetadata type.
type FilenameMetadata = iscanner.FilenameMetadata

// GetBookDataFromDir extracts metadata from a relative directory path.
func GetBookDataFromDir(relPath string) *FilenameMetadata {
	return iscanner.GetBookDataFromDir(relPath)
}
