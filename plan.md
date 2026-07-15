# Plan: Audiobookshelf Go Port - Library/Home UI Sizing Controls Regression Fix

1. **Bug Identification**:
   - The card/shelf sizing control slider is hidden on the Home page (`/`) if the user has previously toggled the Library view to "List View" (which sets `library-style` to `list` in localStorage). Since the Home page always uses bookshelf rows (not a table), the size controls should remain visible.

2. **Proposed Modifications**:
   - Update `frontend/js/app.js` in `navigateTo` and `setStyle` to ensure the shelf size controls are always shown on the Home page (`relPath === '/'`) regardless of the selected `library-style`.

3. **Rebuild & Verification**:
   - Run `go run run.go build` to rebuild assets and compile the server.
   - Run `go run run.go test` to verify the baseline remains green.
