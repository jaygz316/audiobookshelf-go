# Task: Author Metadata Scraping & Matching Enhancement

## Objective
Improve and enhance the Author Metadata Scraping & Matching feature with:
1. **ASIN Direct Query**: Enable search by a 10-character ASIN directly in the search field to bypass name matching and fetch details immediately.
2. **Author Image Deletion**: Implement a `DELETE /api/authors/{id}/image` endpoint on the Go backend to delete the profile photo file and clear it from the database.
3. **Frontend Management**:
   - Add a fallback placeholder image if an author's image fails to load.
   - Embed a "Remove Image" button in the Edit Author modal allowing admins to delete the profile picture directly.

## Proposed Changes

### 1. Go Backend: ASIN Detection in Search
- Update `SearchAuthors` in `internal/finders/finders.go` to detect if the query is a valid 10-character ASIN.
- If it is a valid ASIN, directly request the author details from Audnexus instead of listing matches.

### 2. Go Backend: Delete Image Endpoint
- Register a DELETE handler for `/api/authors/{id}/image` in `handleAuthorsDispatch` inside `internal/handlers/routes.go`.
- Implement `handleDeleteAuthorImage` in `internal/handlers/authors_series.go` to:
  - Verify admin/root privileges.
  - Delete the physical image file from the `<metadata-path>/authors` directory.
  - Clear the `imagePath` in the database for the author.

### 3. Frontend UI Updates
- Update `loadAuthorDetails` in `frontend/js/authors.js` to handle image loading errors gracefully on the details page.
- Update `openEditAuthorModal` in `frontend/js/authors.js` to render the author image and provide a "Remove Image" action if a profile image is present.

### 4. Verification
- Create unit tests for deleting author images and searching via ASIN.
