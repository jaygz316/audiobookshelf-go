# Plan: Visual Mirroring & Header Layout Symmetry

## Frontend Changes
1. **frontend/index.html**:
   - Change search input placeholder to `"Search.."`.
   - Add cast button, server stats/activity button, upload button, and settings gear button in the top-right header section.
   - Modify user profile button layout into a unified pill showing username alongside user initials.

2. **frontend/js/app.js**:
   - Set the username text in the header user badge.
   - Remove the old text-based scan/upload buttons from the left dropdown area (to avoid duplicate upload handlers) and wire the new header icon buttons (`header-upload-btn`, `header-settings-btn`, `header-activity-btn`).
   - Manage the visibility (`hidden` toggle) of these admin buttons depending on the user permissions.

3. **frontend/css/styles.css**:
   - Adjust `.bookshelfRow` items container alignment to `align-items: flex-end` so cards rest flush against the shelf divider rather than floating vertically centered.

## Verification
- Build and run the Go backend, verify that everything compiles.
- Run Go unit/e2e tests.
