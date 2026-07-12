# Task: Smart Collections Implementation (Completed)

## Objective
Implement **Smart Collections**, which automatically group library books based on user-defined dynamic rules (genres, tags, authors, series, narrators, search query) rather than manual curation.

## Proposed Changes

### 1. Database Schema Migration
- [x] In `internal/db/db.go`:
  - [x] Update `bootstrapSchema` to create the `collections` table with two additional columns: `isSmart INTEGER DEFAULT 0` and `rules TEXT` (JSON representation of the rules).
  - [x] Update `migrateDatabase` to check if `isSmart` and `rules` columns exist in `collections` table, and run `ALTER TABLE collections ADD COLUMN ...` if they do not.

### 2. Playlist Manager & Backend Logic
- [x] In `internal/playlist/playlist.go`:
  - [x] Update `Collection` struct to add `IsSmart bool json:"isSmart"` and `Rules string json:"rules"`.
  - [x] In `CreateCollection` and `UpdateCollection`, read and write `isSmart` and `rules` to the database.
  - [x] In `GetCollection`, read `isSmart` and `rules`. If `isSmart` is true, dynamically query matched item IDs from the database using a helper `ResolveSmartCollectionItems` instead of querying from `collectionBooks`.
  - [x] Implement `ResolveSmartCollectionItems` which:
    - [x] Parses the JSON rules.
    - [x] Dynamically builds a SQLite query with parameter bindings to fetch matching `books.id` values.
    - [x] Supports rules for `genres`, `tags`, `authors`, `series`, `narrators`, and `search`.

### 3. API Handlers Update
- [x] In `internal/handlers/playlist_handlers.go`:
  - [x] Update `queryCollectionsForLibrary` to query `isSmart` and `rules`. If `isSmart` is true, dynamically evaluate the rules using `ResolveSmartCollectionItems` (or the equivalent logic) to populate the collections' book lists.
  - [x] Update `handleCreateCollection` and `handleUpdateCollection` to parse `isSmart` and `rules` from the JSON request payload and save them.

### 4. Frontend UI Updates
- [x] In `frontend/js/collections.js`:
  - [x] Add a "Smart Collection" toggle in the Create and Edit Collection modals.
  - [x] If "Smart Collection" is enabled, hide the manual books selector checklist and instead display rule inputs:
    - [x] Genres (comma-separated list)
    - [x] Tags (comma-separated list)
    - [x] Narrators (comma-separated list)
    - [x] Search Query (text input)
  - [x] Display a "Smart" badge on smart collection cards in the grid.
  - [x] For smart collections in the detail view:
    - [x] Display a banner/badge noting that it is a smart/dynamic collection.
    - [x] Hide manual item reordering/removal buttons (Up, Down, Close) since the membership is dynamically generated.

### 5. Verification
- [x] Add integration/unit tests in `internal/playlist/playlist_test.go` and `internal/handlers/playlist_handlers_test.go` to test creating, updating, resolving, and deleting smart collections with various rules.
- [x] Run tests and verify success.
