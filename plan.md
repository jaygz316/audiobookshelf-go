# Plan: Libraries List View UI Alignment

1. **Frontend Settings JS Refactor (`frontend/js/settings.js`)**:
   - Determine active library status via `lib.id === getActiveLibraryId()`.
   - Use `border-l-warning` (orange) for selected items and `border-l-black-400` (gray) for unselected items.
   - Replace flat Edit/Delete buttons with a three-dot vertical menu dropdown (`more_vert`) toggleable via click handler, closing other dropdowns on open and click-outside.
   - Maintain the reorder drag handle (`drag_handle`) and standard `Scan` button alignment.

2. **Styles CSS Refactor (`frontend/css/styles.css`)**:
   - Add utility classes: `.border-l-warning` and `.border-l-black-400`.
   - Add dropdown support styles for `.library-actions-menu` if needed to ensure correct positioning.

3. **Verify Build and Tests**:
   - Go mod tidy, build, and test.
   - Docker build and push.
