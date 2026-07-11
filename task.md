# Task: Author Metadata Scraping & Matching

## Objective
Implement Author Metadata Scraping & Matching, enabling users to search for author details using external metadata providers (like Audnexus) and apply the match (ASIN, biography/description, profile photo) to local author entries in Audiobookshelf.

## Proposed Changes

### 1. Extend the Search API
- [x] Add `GET /api/search/authors` endpoint:
  - [x] Query parameters: `provider` (defaults to "audnexus"), `name` (author name to search).
  - [x] Searches Audnexus API for authors matching the query.
  - [x] Concurrently resolves details (description, image URL) for top 5 matches to show detailed information in the UI search list.
  - [x] Registers the route `/api/search/authors` in `registerSearchRoutes` inside `internal/handlers/routes.go`.
  - [x] Implements `handleSearchAuthors` in `internal/handlers/search_handlers.go`.
  - [x] Implements `SearchAuthors` inside `internal/finders/finders.go`.

### 2. Implement Author Matching Backend
- [x] Update `handleMatchAuthor` in `internal/handlers/authors_series.go`:
  - [x] Accepts a JSON payload: `{ "asin": "...", "provider": "..." }`.
  - [x] Queries `AudnexusProvider` to retrieve the author details (Name, ASIN, Description, Image URL).
  - [x] Automatically downloads the author profile image from the retrieved image URL and saves it to `<metadata-path>/authors/<authorID>.jpg`.
  - [x] Updates the SQLite database record for the author with the new `asin`, `description` (biography), `imagePath` (`authors/<authorID>.jpg`), and `updatedAt`.
  - [x] Formats `lastFirst` from the author name if it was not previously set.
  - [x] Refreshes/updates the linked `libraryItems` table caches for author names.
  - [x] Updates the signature to accept `cfg *core.Config` to know the metadata folder path.

### 3. Build the Frontend UI
- [x] Update `openEditAuthorModal` in `frontend/js/authors.js`:
  - [x] Add a **Match** button to open the Author Match Modal.
- [x] Create an interactive **Author Match Modal**:
  - [x] Allows selecting the metadata provider (Audnexus).
  - [x] Pre-fills search inputs with the author's name.
  - [x] Displays search results with biography previews and author photos.
  - [x] On clicking "Match/Import", triggers `POST /api/authors/<ID>/match` to apply the metadata and download the image.
  - [x] Triggers a success notification and refreshes the author detail view.

### 4. Tests & Verification
- [x] Create `internal/handlers/authors_match_test.go` to unit-test the search and match handlers with mocked external API requests.
- [x] Verify through E2E execution.

