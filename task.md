# Audiobookshelf Go Rewrite: Series and Author Bundling Tasks

## Goal
Implement a chronological series matrix on the series details page that groups books by their sequence number/position, allowing the user to cleanly see and track multiple narrator versions of the same title side-by-side or stacked.

## Implementation Details
1. **Frontend Update** (`frontend/js/authors.js`):
   - Modify the `loadSeriesDetails` function.
   - Group the series items dynamically by their sequence.
   - Sort sequences numerically.
   - For each sequence, render a list of narrator versions side-by-side/stacked with their specific title, narrator details, publisher, and duration comparison.
   - Add visual cues like book badges, schedule/duration indicators, and clean layout with modern colors.
2. **Verification & Testing**:
   - Re-build the frontend with `npm run generate`.
   - Run tests `go test ./...` and `go test ./e2e/...` to verify nothing is broken.
