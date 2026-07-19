# Plan - Sidebar & Header Navigation Visually Aligned

We are aligning the left navigation rail, sidebar aesthetics, and top bar search bar layout to match the original Audiobookshelf web client.

## Planned Changes

### 1. Sidebar Nav Links Refactoring
- Simplify class mutation in `frontend/js/router.js` (`highlightSidebarLink`) by toggling a clean `.active` class on the link element.
- Define unified sidebar navigation links styling in `frontend/css/layout.css` and `frontend/css/components.css`.
- Remove horizontal borders (`border-b`) between sidebar link elements to match the original client's layout.
- Ensure the sidebar background on desktop and mobile uses the primary dark theme color (`--color-primary`) to establish a clean container separation from the main viewport's background.

### 2. Search Bar Visual Alignment
- Reposition the search magnifying glass icon to the left inside the input field, using absolute positioning.
- Make the clear (close) button a distinct button on the right side of the search input, hidden by default, and shown only when characters are typed.
- Update `frontend/js/app.js` to toggle the visibility of the right clear button and handle clearing logic seamlessly.

### 3. Verify & Test
- Format, vet, compile, and run Go test suites to verify zero regressions.
