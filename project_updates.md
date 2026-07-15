# Project Updates & Deprecated Patterns

> **Purpose**: This file acts as a living document to track architectural decisions, deprecations, and recent updates. **AI agents MUST read this file on initialization** to ensure they do not write stale, obsolete, or incorrect code.

---

## 🚫 Deprecated / Do Not Use List

| Pattern/Library | Replacement | Context & Reason |
| :--- | :--- | :--- |
| `github.com/mattn/go-sqlite3` | `modernc.org/sqlite` | We are CGO-free. Always use `modernc.org/sqlite` for database operations. |
| External Web Frameworks (Gin, Echo, Fiber) | `net/http` (Go standard library) | The backend uses standard Go `http.ServeMux`. Do not import or transition to routing frameworks. |
| Raw WebSockets (`gorilla/websocket`) | `github.com/zishang520/socket.io/v2` | Socket.io v2 is the standard for client-server real-time updates and presence. gorilla/websocket is only for testing/low-level compatibility. |
| Server-rendered templates (HTML/Go templates) | Static Vue/Nuxt SPA (`frontend/` directory) | All UI routes and templates must remain on the static Nuxt client. The Go binary embeds this via `//go:embed`. |
| Dynamic Server-side Config Writes | Standard config directories (`--config` flag) | Do not hardcode or dynamically write config paths to `/tmp` or parent dirs. Respect configuration settings. |

---

## 🏗️ Active Core Technologies & Conventions

1. **Backend**: Go using pure stdlib routing, thread-safe customized SQLite querying, and Socket.io for notifications/sync.
2. **Frontend**: Nuxt.js/Vue.js static SPA built into `frontend/` and embedded inside Go code.
3. **Database Migrations**: Handled in `internal/db/`. New schemas must follow the existing migration file pattern.
4. **Real-time Engine**: Powered by `zishang520/engine.io` / `socket.io` to support original client presence/progress sync.

---

## 📅 Log of Recent Updates & Deprecations

*This log is updated by developers/agents whenever an API, design pattern, or library is deprecated or updated.*

### 2026-07-14
- **Established Project Updates Tracker**: Created `project_updates.md` and integrated it into the startup check of `AGENTS.md`, `scheduled_prompt.md`, and `ux_scheduled_prompt.md`.
- **Sidebar & Siderail Navigation Styling**: Switched navigation styling to exactly match the original client's layout (`bg-bg` and custom icons). Removed custom template overrides in favor of native CSS styles.

### 2026-07-15
- **Item Details UX Enhancement**: Implemented visual parity features on the Item Details view matching the original client. Added "Add to Playlist", "Download", and "Delete" actions (the latter is admin-restricted). Created and wired up a dedicated, interactive "Listening/Reading Progress" card to track, reset, and toggle item completion status.
- **Series View UX Audit & Parity**:
  - Re-styled and optimized the Series stacked cover grid.
  - Linked the Series list grid column size and series cover card dimensions to the `--bookshelf-card-width` CSS variable so that it responds dynamically to the shelf size control slider.
  - Allowed the shelf size control slider to be visible on `/series` and `/authors` list pages.
  - Added seamless Back to Series and Back to Authors navigation links.
  - Implemented real-time reading progress cache querying for series cards, displaying an overall progress bar matching Vue.js frontend standards.
- **Audio Player UI Parity & Chapter Controls**:
  - Implemented Previous/Next Chapter navigation and queue item transitions matching original Vue component behavior.
  - Added a chapters list modal (`player-chapters-dialog`) triggered by `player-chapters-btn` displaying start times and durations, with auto-scroll to the active chapter.
  - Implemented custom skip forward/backward duration selects in the Player Settings modal, dynamically saving to/loading from `localStorage` and updating seek button icons and tooltips.
  - Added chapter titles into the timeline hover tooltips and dynamic chapter info display (`player-chapter-info`) below the scrubber.
- **Collections & Playlists Cover Fallbacks**: Fixed invalid `/assets/cover-fallback.png` image references in collections and playlist views, changing them to use the existing `assets/images/book_placeholder.jpg` to prevent 404 resource errors and broken cover icons in the UI.
- **Podcast View & Episodes Audit**: Completed detailed visual parity and functional audit of the Podcast view. Verified iTunes/RSS subscription actions, filter and sorting mechanisms, settings updates, downloading/queueing tasks, and episode-specific actions (play/resume, mark played/unplayed, delete, hard delete). Ran Go end-to-end integration tests to verify API correctness.
- **Authors & Playlists UX Parity (Priority 10)**:
  - Added search input, sort fields (Name/Book Count), and sort order controls to the Authors page, matching the Narrators page UI. Added backend support for search querying in GET `/api/libraries/:id/authors`.
  - Added a "Play Playlist" button to the Playlist details page header.
  - Implemented `playItems` in `player.js` to clear/populate the playbackQueue with playlist items and play them sequentially starting from any index.
- **Settings Page Bookmarkable Tab Hashes**: Enhanced the Settings page tab switcher to synchronize with and initialize from `window.location.hash`, preserving the active settings sub-tab selection (Users, Libraries, Server, Auth, etc.) on page refreshes and bookmark links.
- **Onboarding Welcome Screen**: Implemented `showNoLibrariesWelcome()` onboarding welcome screen for new users or setups with empty libraries, offering a direct "Add Your First Library" shortcut button that links straight to the Settings Libraries sub-tab and launches the creation dialog.
- **FS Directory Picker**: Integrated an interactive folder browse modal overlay into the library configuration modal, querying GET `/api/filesystem` to list server directories, drill down into subfolders, and navigate up.
- **Listening Stats Tabs, Line Charts, and Heatmaps**: Completed visual/functional audit of the Stats page. Re-implemented `stats.js` to feature tabs: "My Stats" (with personal statistics, SVG-based line chart, streak tracker, and interactive year-wide heatmap calendar with CSS tooltips), "Library Stats" (with metadata cards and progress-bar based lists for Genres, Authors, Longest, and Largest items), and "Server Stats" (with paginated playback sessions table and aggregated bar charts). All tests passing.
- **Upload Page & Drag-and-Drop UX Fixes (Priority 12)**:
  - Enabled dynamic conditional display of `header-upload-btn` and `upload-btn` (and associated dragover/drop upload listeners) matching user upload permission (`user.permissions.upload`).
  - Fixed standard directory batch recursion limitation in `webkitGetAsEntry` drop/drag upload parsing (`getFilesFromEntry` in `upload.js` and `app.js`) to support folders containing over 100 files by looping through `dirReader.readEntries()`.
  - Unhidden the "Clear All" queue button in the upload modal when files are added to the queue, and wired up its dynamic visibility state in `updateQueueUI()`.
- **E-book Reader UX Fixes (Priority 13)**:
  - Fixed typography scale persistence on reader initialization so that when a user adjusts font scale and refreshes the reader view, the correct `currentFontSize` configuration is selected and set on the EPUB rendition instance.
  - Fixed bookmark deletion highlight removal logic by passing `"highlight"` as the specific annotation type parameter to `rendition.annotations.remove` instead of mapping custom color CSS classes.
  - Integrated layout tracking support in the reader footer by querying `book.locations.locationFromCfi` indices and updating `pageInfo.textContent` to show `"Page X of Y"` pagination indices.
- **Login & Initial Setup Screen Audit**: Added a `login-custom-message` banner to support server-provided custom messages, updated `frontend/js/auth.js` to toggle visibility/content based on status response data, and prioritized the server-provided `authOpenIDButtonText` property for OIDC button text.
- **Upload Permissions & Button Decoupling**: Decoupled the header upload and global upload button configurations from the admin-only check in `frontend/js/app.js` to correctly honor granular user upload permissions.
