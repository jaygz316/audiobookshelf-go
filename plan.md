# Implementation Plan - Batch Metadata Editing

We will implement Batch Metadata Editing in the Go backend and Vue/Nuxt static frontend UI to achieve feature parity with the original Audiobookshelf.

## Proposed Changes

1. **Backend REST API Endpoints**
   - Create a new endpoint `POST /api/items/batch/update`:
     - Authenticated: Admin/Root roles only.
     - Payload: JSON array of objects, e.g.:
       ```json
       [
         {
           "id": "library-item-id",
           "mediaPayload": {
             "tags": ["TagA"],
             "genres": ["GenreB"],
             "explicit": true
           }
         }
       ]
       ```
     - For each item, retrieve its current metadata, merge with the fields supplied in `mediaPayload` (using pointers to detect specified/omitted fields), perform the updates on tables `books`/`podcasts`, `libraryItems`, `bookAuthors`, `bookSeries`, and write updated metadata JSON if `MetadataMarkdownWithItem` is enabled.
     - Broadcast a Socket.io `item_updated` event to sync the UI in real-time.

2. **Frontend UI Integration**
   - Add a "Batch Edit" toggle button in the library header/toolbar.
   - When active:
     - Render checkboxes/borders on the audiobook/podcast cards.
     - Clicking a card toggles its selection instead of showing details.
     - Show a floating bottom bar: "X items selected", with buttons "Edit Metadata" and "Cancel".
     - Clicking "Edit Metadata" opens a beautiful modal listing editable fields (Tags, Genres, Author, Narrators, Series, Explicit, Abridged, Publisher, Published Year) with checkboxes next to each field to toggle its inclusion in the batch update.
     - Clicking "Save" calls the backend API and refreshes the library view.

3. **Verification**
   - Unit tests in `internal/handlers/batch_edit_test.go` checking the batch update endpoint.
   - E2E integration test in `e2e/f13_batch_edit_test.go`.
