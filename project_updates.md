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
- **Mobile Navigation Responsive Toggles & Smooth Transitions**:
  - Implemented mobile menu drawer backdrop overlay `#sidebar-backdrop` to dim and blur main layout viewports on mobile size viewports.
  - Re-implemented the Mobile Sidebar Drawer navigation layout to slide in and out smoothly from the left using CSS transform transitions (`translateX(-100%)` to `translateX(0)`) and visibility overrides instead of block class swaps.
  - Styled active navigation page indicators to scale (`scaleY(0)` to `scaleY(1)`) and fade (`opacity: 0` to `opacity: 1`) smoothly using CSS transitions.
  - Linked backdrop overlay click events and navigation item selection handlers to automatically invoke the drawer's slide-out transition and visibility timers.
- **Home/Dashboard & Task Runner UX Audit**:
  - Added `bookshelf-card` class to dynamically generated library cards to ensure CSS selectors map perfectly.
  - Refactored `initBatchEditHandlers` in `dashboard.js` to target `.bookshelf-card` elements instead of `.bookshelfRow .group`, ensuring batch editing styling applies correctly to the bottom grid shelf ("All Books") and all other list/grid elements.
  - Added `cursor-pointer` utility classes to shelf sizing control dec/inc buttons in `index.html` for clean hover cursor indicators.
  - Updated Go task runner `runVet` command in `run.go` to vet native and WebAssembly packages separately using their respective build constraints, resolving validation compile errors.
- **Item Details UX Enhancement**: Implemented visual parity features on the Item Details view matching the original client. Added "Add to Playlist", "Download", and "Delete" actions (the latter is admin-restricted). Created and wired up a dedicated, interactive "Listening/Reading Progress" card to track, reset, and toggle item completion status.
- **Series View UX Audit & Parity**:
  - Re-styled and optimized the Series stacked cover grid.
  - Linked the Series list grid column size and series cover card dimensions to the `--bookshelf-card-width` CSS variable so that it responds dynamically to the shelf size control slider.
  - Allowed the shelf size control slider to be visible on `/series` and `/authors` list pages.
  - Added seamless Back to Series and Back to Authors navigation links.
  - Implemented real-time reading progress cache querying for series cards, displaying an overall progress bar matching Vue.js frontend standards.
  - Fixed dynamic card scaling on library grid view and fanned series stack sizing constraint by updating CSS rules in `styles.css` to respond to the sizing slider.
  - Removed redundant `mr-8e` class from the default card class list in `dashboard.js` to prevent uneven spacing in grid and flex layout containers.
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
- **Playlist Track Playback & Queue Optimization**: Added a track-level play button (`play-track-btn`) inside each playlist track item's action container, allowing starting playlist playback from that specific track, and corrected `playItems` queue slicing logic in `player.js` to populate the `playbackQueue` with subsequent tracks, preventing out-of-order repeats.
- **Mobile Responsive Navigation Menu**: Added a mobile menu hamburger button in the header and configured the Left Navigation Sidebar with an ID and stateful toggling logic (including auto-close on navigation selection, click-outside dismissal, and resize-to-desktop styling reset) in `frontend/js/app.js` to ensure visual and navigation parity for mobile screen layout.
- **Header Bar Audit & Notification Tasks Widget**:
  - Implemented the notifications bell/spinner tasks widget in the header bar matching `widgets-notification-widget.vue`. It queries `/api/tasks` periodically (every 10 seconds), showing a spinning icon when tasks are running, or a standard bell icon when idle, along with an unseen success badge (green dot with ping animation) for unseen completed/failed tasks.
  - Wired settings/administration links in the user dropdown. "Settings" routes to `/settings` and "Administration" routes to `/settings#users`.
  - Registered `'header-notification-dropdown'` in the global `closeAllDropdowns` list.
- **Library Modal & Directory Picker fixes (Priority 5)**:
  - Updated `openFolderPicker` template in `settings.js` to use defined Tailwind classes (`border-black-300`, `bg-black-500`, `divide-black-400`).
  - Fixed a bug in the directory picker where navigation (double-click/up button) permanently disabled the "Select Folder" button, allowing selection of the currently viewed directory.
  - Enhanced backend update handlers (`HandleUpdateLibrary` in `library_handlers.go` and `UpdateLibrary` in `db_queries.go`) to validate, resolve, and update modified folder paths for existing libraries instead of silently ignoring edits on existing rows.
  - **EPUB Iframe Style Injection & Render Hooks**: Implemented `applyIframeStyles()` inside `reader.js` to dynamically inject universal selector CSS rules (`*` color, background-color, line-height) into the EPUB rendition iframe contents upon theme selection and loading. Registered a listener for the rendition's `rendered` event hook to automatically style new chapters/sections as they load.
  - **Safe Floating-Point Comparisons for Bookmarks**: Replaced direct float64 equivalence checks (`==`) in `internal/handlers/me.go` bookmark update/delete handlers with a safe tolerance diff check (`diff < 0.001`), preventing deletion failures due to floating-point parsing precision mismatch.
- **Audio Player & Dashboard Parameter Resolution**: Fixed a critical parameter type mismatch bug in `playItem` where playing items from the dashboard passed a string ID rather than an object. Updated `playItem` to consistently use the resolved `itemObj` metadata object for speed retrieval, metadata updating, and Cast playback.
- **Sidebar Footer & Dynamic Version Info**:
  - Dynamically load the server version (`sidebar-version`) and environment source (`sidebar-source`) from `/status` and authorization response payloads, removing hardcoded indicators.
  - Wired up the help button (`sidebar-help-btn`) and version link (`sidebar-version`) in the footer to open the official documentation and original releases page in a new window/tab.
- **Login & Initial Setup Screen Audit (Priority 1 - Regression Pass)**:
  - Added relative `.password-wrapper` containers and visibility toggles (`.password-toggle-btn` with eye icons) to the login password input, setup password input, and setup confirm password input.
  - Implemented CSS styles in `styles.css` to make the toggle buttons visible when the wrapper is hovered or contains focus.
  - Wired up click event handlers in `setupEventHandlers()` in `app.js` to toggle password visibility.
  - Implemented a client-side HTML sanitizer `sanitizeHTML()` in `auth.js` to securely filter server-provided custom login messages before rendering them in the DOM to prevent Stored XSS.
- **Go Task Runner Implementation**: Rewrote the non-Go build and startup scripts (`Makefile` and `start.sh`) into a unified, cross-platform task runner written in pure Go (`run.go`). The `Makefile` and `start.sh` files were updated to thin delegators to `run.go`, preserving backward compatibility while bringing all build, testing, linting, formatting check, and startup logic into Go.
- **Go WebAssembly Frontend Core Rewrite**:
  - Rewrote the core frontend logic (authentication checks, OIDC single-sign-on integration, first-run initialization setup cards, and DOM-based HTML sanitization) in Go, located in [main.go](file:///home/jay/projects/audiobookshelf-go/frontend/go/main.go) and compiled to WebAssembly (`frontend/main.wasm`).
  - Configured [index.html](file:///home/jay/projects/audiobookshelf-go/frontend/index.html) to initialize the WASM bundle via a global `window.wasmReady` Promise, avoiding race conditions with page loading.
  - Rewrote [auth.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/auth.js) as a bridge delegating to the Go WebAssembly exported functions.
  - Exposed `request` and `resolvePath` utilities globally as `window.apiRequest` and `window.resolvePath` to allow the Go WebAssembly client to execute network requests.
  - Updated [run.go](file:///home/jay/projects/audiobookshelf-go/run.go) to automatically compile `main.wasm` and copy Go's official `wasm_exec.js` runtime utility during compilation. Adjusted the task runner's `test` command to explicitly target test directories to prevent native compilation issues on WebAssembly constraints.
- **Home/Dashboard Bookshelf Texture Scaling Fix**: Added `background-size: auto 100% !important` style to `.bookshelfRow` in [styles.css](file:///home/jay/projects/audiobookshelf-go/frontend/css/styles.css) to ensure the wooden shelf background textures scale nicely with dynamic row heights when adjusted by the card-size control slider.
- **Home/Dashboard Bookshelf Visual Parity & Reflections**: Layered the shelf divider plank and wood texture in the background of `.bookshelfRow` in [styles.css](file:///home/jay/projects/audiobookshelf-go/frontend/css/styles.css). Adjusted bottom padding to 20px so books rest directly on the shelf and their cover reflections draw seamlessly over the divider. Hidden the redundant `.bookshelfDividerCategorized` element and offset `.categoryPlacard` up by 8px to center on the shelf.
- **Onboarding Setup Wizard Enhancement**: Transitioned the initial server setup screen into a robust 3-step wizard workflow (Account Setup -> Library Configuration -> Summary Confirmation). Re-implemented `showSetupScreenGo` inside [main.go](file:///home/jay/projects/audiobookshelf-go/frontend/go/main.go) in Go WebAssembly to handle stepping forward/backward, validating input values per step, dynamically updating progress indicators, showing a summary, and performing a sequential POST registration, login, token storage, and library creation payload submission flow.
- **Dynamic Sort Dropdown & Filter Fixes**:
  - Refactored `initCustomFilterAndSort` in [app.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/app.js) to dynamically populate sorting options and update selected labels depending on the active library's media type (e.g. Title, Author, Year, Date Added, Duration for books vs. Title, Publisher, Date Added, Episodes, Random for podcasts).
  - Wired up a global listener on the `library-changed` event to update the sorting menu layout and validate/map stale stored sort settings in `localStorage` when switching libraries.
  - Corrected checks in [dashboard.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/dashboard.js) to query `activeFilter` instead of the function parameter `filterBy`. This fixes rendering errors where personalized shelves were incorrectly shown when a filter was set via `localStorage` on page reload.
  - Fixed `TestGetLibraryPersonalized` in [main_test.go](file:///home/jay/projects/audiobookshelf-go/main_test.go) to dynamically search for the `recently-added` shelf inside the personalized shelves payload rather than assuming it's the only shelf, ensuring robustness as shelves like `Discover` are added.
  - Enabled the shelf card size control widget to be visible on the Home/Dashboard view (`/` path) in [app.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/app.js).
  - Dynamically labeled the "Author" column as "Publisher" for podcast libraries in both list view table headers and list view column customization options in [dashboard.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/dashboard.js).
- **UI Parity & Settings Hover Fixes**:
  - Audited and verified sorting/filtering menu UI/UX styling, header/sidebar layout symmetry, and wooden bookshelf texture scaling.
  - Fixed non-existent CSS hover color classes (`hover:bg-black-450` and `hover:bg-black-350`) in `settings.js` by aligning them with correct design tokens (`hover:bg-black-400` and `hover:bg-black-300`), which restores functional hover highlights to library rows, cancel buttons, and action menu triggers.
  - Verified drag-and-drop reordering, active library borders, and switches.
  - Rebuilt assets and verified that Go unit, integration, and vet test suites pass.
- **Audio Player Keyboard Controls & MediaSession Integration**:
  - Implemented global keyboard shortcuts (Space to toggle play/pause, ArrowLeft/Right to seek back and forward) that bypass triggering when focus is in input fields or textareas.
  - Added support for the browser's expansion of MediaSession API, enabling system-level/lock screen controls, headphone controls, and status sync.
  - Exposed media control handlers in `player.js` to handle play, pause, seek backward, seek forward, previous chapter, and next chapter/queue transitions.
  - Automatically updates the OS media session metadata (title, artist/author, album, cover image) and playbackState ('playing', 'paused', 'none') dynamically on playback events.
  - Built WebAssembly assets and verified backend tests successfully.
- **Results Header Count Pluralization & Admin Paths**:
  - Audited the results header and corrected item count pluralization logic across `dashboard.js`, `podcasts.js`, `collections.js`, and `playlists.js` so that counts correctly use singular forms (e.g. `1 Book`, `1 Active Task`, `1 Collection`, `1 Playlist`) instead of forced plurals.
  - Enhanced the Item Details metadata grid in `itemDetails.js` to display the absolute file/folder path on disk when the logged-in user is an administrator (`isAdmin` context flag).
  - Added `//go:build js && wasm` build tag to `frontend/go/main.go` to cleanly segregate WebAssembly dependencies during native environment test execution and compilation runs.
- **Library Toolbar Filter & Item Details Badges Navigation**:
  - Implemented an interactive inline "Clear Filter" close icon button next to the filter dropdown button in the main library toolbar, toggling visibility based on active filter state.
  - Exposed `window.updateFilterLabelGlobal` in `app.js` and updated `loadDashboard` in `dashboard.js` to save filter settings to local storage when dynamically navigated and update the toolbar labels automatically.
  - Styled genre and tag badges on the Item Details page as interactive links with transition properties, binding them to trigger custom events filtering the main dashboard view.

### 2026-07-16
- **Shelf Sizing Slider Consistency**: Fixed a bug where the bookshelf card sizing slider (`shelf-size-control`) would be hidden on grid-based views like `/series`, `/authors`, `/collections`, `/playlists`, and `/narrators` if the user had selected the `list` view style on the main `/library` page. The visibility logic was corrected to ensure that the slider remains visible on all grid-based pages regardless of the main library view style.
- **Sidebar Active Indicator Transitions**: Refactored the sidebar highlight logic in `frontend/js/app.js` to remove manual toggles of the `hidden` class on the `.active-indicator` element. This allows the CSS-defined transform (`scaleY`) and opacity transitions to play smoothly when navigating between sections.
- **Mobile Touch Indicators & Tactile Active States**: Added responsive active press states (`:active`) in [styles.css](file:///home/jay/projects/audiobookshelf-go/frontend/css/styles.css) for interactive navigation elements (sidebar links, mobile menu buttons, library selectors, scan buttons). This provides immediate visual scale-down and opacity feedback on touch devices to improve mobile UX.
- **Settings Sidebar & Tabs Parity**: Reordered settings sub-navigation links to match original Audiobookshelf organization, renamed labels to official names (e.g. "Playback Sessions", "Custom Metadata Providers", "System Logs"), and set the default active tab on first load to "Libraries" with correct class and content panel mapping.
- **Authentication Settings Manual OIDC Fields**: Added manual URL and key configuration fields (`authOpenIDAuthorizationURL`, `authOpenIDTokenURL`, `authOpenIDUserInfoURL`, `authOpenIDJwksURL`, `authOpenIDLogoutURL`, and `authOpenIDTokenSigningAlgorithm`) to backend `OIDCSettings` in [auth.go](file:///home/jay/projects/audiobookshelf-go/internal/auth/auth.go). Configured them to bypass dynamic issuer discovery in `HandleLogin` and `HandleCallback` if manually set, and manually verify tokens using RemoteKeySet with custom signing algorithms if JwksURL is configured.
- **OIDC Manual Fields API Integration**: Updated `getOIDCSettings` in [oidc_handlers.go](file:///home/jay/projects/audiobookshelf-go/internal/handlers/oidc_handlers.go) and the OIDC initialization check in [routes.go](file:///home/jay/projects/audiobookshelf-go/internal/handlers/routes.go) to load and map these settings.
- **RSS Feeds Settings Panel Visual Parity Audit**:
  - Polished the RSS feeds settings layout with custom material icons mapped to entity type (`book`, `podcast`, `playlist`, `collection`, `series`, default `rss_feed`).
  - Added capitalized entity type labels (`Book`, `Podcast`, etc.) for improved aesthetic parity.
  - Refactored copy URL and delete actions to show premium toast notification alerts (`showToast`) instead of native browser popups.
  - Fixed a potential bug by importing `showToast` from `./app.js` to `settings.js` and `itemDetails.js`, preventing runtime ReferenceError crashes.
  - Re-routed tab click logic to dynamically call `renderFeedsTab()` when navigating to the feeds tab.
  - Integrated `showToast` for all public RSS actions inside `collections.js` and `itemDetails.js` details panels.
- **E-Reader Email Settings Visual & Behavioral Parity Audit**:
  - Replaced browser `alert()` popups in `renderEmailsTab()`, `renderEreaderDevicesRows()`, and `triggerEreaderDeviceModal()` with premium `showToast(...)` toast notifications.
  - Added Material Symbols (`save`, `mail`, `devices`) to SMTP Save Settings, Send Test Email, and Add Device buttons.
  - Introduced a dynamic CSS-based sending/loading spinner inside the Send Test Email button.
  - Redesigned E-Reader device list rows' Edit/Delete actions using inline-flex icon buttons with `edit` and `delete` Material Symbol icons.
  - Polished the modal footer actions inside the Add/Edit E-Reader Device modal (`triggerEreaderDeviceModal`) to use standard `close` and `check` Material Symbol icons and transition animations.
  - Resolved dynamic selector scoping issues in device row event listeners by replacing `e.target` dataset lookups with `e.currentTarget` dataset lookups, ensuring compatibility with nested span/icon child elements.
- **Library Grid, Count, & Toolbar Alignment**:
  - Removed `font-mono` typography classes from the toolbar results count label (`#book-count`) and separators (`#view-title-separator`) in `frontend/index.html` to align with the clean, modern sans-serif typography of the original project.
  - Refined custom Filter and Sort dropdown menu items' hover colors in `frontend/js/app.js` from `hover:bg-black-500` to `hover:bg-black-400` so that highlights are clearly visible against the dark gray background.
  - Aligned global styles in `frontend/css/styles.css` by updating core variable configurations (`--color-bg` to `#2c2c2c` for true charcoal backgrounds and `--color-accent` to `#e5a93b` for gold highlights) to match the brand color scheme of the original Audiobookshelf project.
- **Series List Reading Progress Optimization**:
  - Optimized the Series view loading speed by adding bulk reading progress queries via direct `mediaProgresses` LEFT JOINs in GET `/api/libraries/:id/series`.
  - Allowed test database compatibility by excluding non-existent mock database columns (`createdAt`) from the `mediaProgresses` table scan within the series list query.
  - Integrated book-level `userProgress` arrays in the JSON response, pre-populating frontend `progressCache` maps in `authors.js` to instantly compute overall series completion percentages.

- **Modularized Database Queries**:
  - Broke down the monolithic `internal/db/db_queries.go` (2,261 lines) into four logically separated files inside the `db` package:
    - [libraries.go](file:///home/jay/.gemini/antigravity/brain/bd7a400e-1bc2-403c-bd6c-929b68d9779c/.system_generated/worktrees/subagent-DB-Query-Modularizer-refactor-agent-14187e60/internal/db/libraries.go): Library management, settings merging, and library/podcast stats.
    - [library_items.go](file:///home/jay/.gemini/antigravity/brain/bd7a400e-1bc2-403c-bd6c-929b68d9779c/.system_generated/worktrees/subagent-DB-Query-Modularizer-refactor-agent-14187e60/internal/db/library_items.go): Library item download info, covers, and minified item retrieval/filtering.
    - [filter_sorting.go](file:///home/jay/.gemini/antigravity/brain/bd7a400e-1bc2-403c-bd6c-929b68d9779c/.system_generated/worktrees/subagent-DB-Query-Modularizer-refactor-agent-14187e60/internal/db/filter_sorting.go): Query filter builders, sorting orders, and library filter metadata.
    - [db_utils.go](file:///home/jay/.gemini/antigravity/brain/bd7a400e-1bc2-403c-bd6c-929b68d9779c/.system_generated/worktrees/subagent-DB-Query-Modularizer-refactor-agent-14187e60/internal/db/db_utils.go): Generic JSON/Epoch and Table existence helper utilities.
  - Removed unused imports and verified all `internal/db` tests pass successfully.

- **Refactored Backup Scheduler & Fixed Challenger Findings**:
  - Implemented dynamic ticker duration inside `internal/backup/task.go` (1s for 6-field crons and 5s for 5-field crons) to prevent missed second-level triggers.
  - Implemented resilient catch-up range evaluation checking each step of resolution `R` from `lastRunTime` to `checkTime` with caps on the maximum catch-up window.
  - Executed `CreateBackup` asynchronously in a background goroutine using `context.Background()` to avoid synchronous blocking of lifecycle operations (`Stop`/`Reload`).
  - Split `scheduler_challenger_test.go` into `scheduler_challenger_test.go` and `scheduler_challenger_stress_test.go` to ensure all files in `internal/backup/` package are strictly under 200 lines.
  - Updated `TestChallengerBlockingLifecycle` to assert that `Stop()` returns instantly (non-blocking) and wait for background backup completion via polling.
  - Resolved a database closure race condition in `TestSchedulerCheckAndRun` by waiting for background task completion using the package-level `BackupRestoreMu` mutex before cleaning up directories.

- **Scheduler Re-initialization & Global DB Mutex Fixes**:
  - Re-initialized the backup scheduler upon database reconnection inside `reconnectDB` in `internal/handlers/backup_handlers.go`, preventing the scheduler from holding closed connections and leaking goroutines.
  - Eliminated data races on the package-level `globalDB` variable in the `handlers` package by wrapping it with a `sync.RWMutex` and implementing thread-safe `GetGlobalDB()` and `SetGlobalDB(...)` functions in `internal/handlers/managers.go`.
  - Replaced all direct reads/writes/comparisons of `globalDB` with thread-safe calls across the handlers files: `backup_handlers.go`, `dispatchers.go`, `managers.go`, `middleware.go`, `routes.go`, `spa.go`, and test files.

### 2026-07-17
- **Refined Shelf Planks, Shadows & Cover Reflections**:
  - Wrapped book cover images and spine creases in a `.book-cover-wrapper` container in `createCard` (`dashboard.js`), ensuring spine creases are included in reflections while play buttons, badges, and hover details remain unreflected.
  - Refactored `.bookshelfRow` (`components.css`) and `.library-shelf-grid` (`layout.css`) backgrounds with a solid back wall gradient (`var(--color-bg)`) overlaying the wood texture behind books, exposing the raw textured wood grain only on the 20px shelf planks.
  - Unified highlights and shadows (`rgba(255,255,255,0.2)` / `rgba(0,0,0,0.35)`) and theme overlays on shelf planks, ensuring wood texture grain and 3D lighting remain visible across themes.
  - Styled category separators (`.bookshelfDividerCategorized`) to utilize the same wood grain texture, highlights, shadows, and theme overlays for visual cohesion.
  - Directed box-shadows, hover elevation (`translateY(-6px)`), and `-webkit-box-reflect` styles to the `.book-cover-wrapper` container to perfectly mirror physical cover styling of the original project.
- **WebAssembly Setup Wizard Modularization**:
  - Modularized `frontend/go/setup.go` by splitting it into three smaller files under 200 lines: `setup.go` (UI screen rendering/transitions), `setup_validation.go` (step validation logic), and `setup_submit.go` (asynchronous submission/API interaction logic).
  - All files retain build tag `//go:build js && wasm` and package `main` declaration, ensuring successful compilation and passing test status.
- **Bookshelf Divider Restoration**:
  - Restored visual display of horizontal wooden divider planks (`.bookshelfDividerCategorized`) beneath categorized bookshelf rows on the Home/Dashboard view, matching the 3D wooden aesthetics of the original client.
- **Bookshelf Sizing Dynamic Row Heights**:
  - Removed static row height (`h-56`) from bookshelf rows inside `dashboard.js` (`createShelfSection`), allowing shelf rows to dynamically scale their heights matching the custom-sized book cards adjusted by the shelf-size range slider.
- **Repeating Shelf Plank Shadows**:
  - Added top drop-shadow linear gradients to both `.bookshelfRow` in `frontend/css/components.css` and `.library-shelf-grid` in `frontend/css/layout.css` to draw repeating wood plank shadows on the wall texture behind the cards.
- **3D Book Cover Spine Crease & Shading**:
  - Injected an absolute crease overlay to book covers inside `createCard` in `frontend/js/dashboard.js`, simulating physical book spines with creases and highlights.
- **Dynamic Reflection Scaling**:
  - Refactored `--bookshelf-reflect` in `frontend/css/variables.css` to use percentage-based fade lengths instead of fixed pixels, ensuring reflections scale dynamically with range slider adjustments.
- **User Profile Pill & Named Material Symbols**:
  - Refactored the user profile dropdown button (`user-menu-btn`) to a beautiful rounded-full pill incorporating a circular initials avatar (`#user-initials`) and a dropdown caret.
  - Converted all hexadecimal icon codepoints in the header (Server Activity, Settings Gear) and sidebar (Latest, Collections, Playlists, Narrators, Stats, Download Queue) to descriptive, standard Material Symbols text ligatures (e.g. `query_stats`, `settings`, `podcasts`, `bookmarks`, `playlist_play`, `record_voice_over`, `bar_chart`, `downloading`), improving code readability and compilation reliability.
  - Standardized book spine crease decoration under a clean `.book-spine-crease` CSS class inside `components.css` and cleaned up inline spine crease styles in `dashboard.js`.
- **Top Appbar Action Buttons & Active Toggle Colors**:
  - Aligned all header action buttons (Notification, Server Activity, Upload, Settings) to use standard, consistent `rounded-full` shapes and `w-9 h-9` proportions with smooth transitions matching original Audiobookshelf styling.
  - Replaced the hardcoded emerald color (`#10b981`) on active switch sliders (`.abs-slider`) with the brand gold accent color variable (`var(--color-accent)`) for premium styling consistency.
- **Settings View Alignments & Responsive Sidebar**:
  - Replaced the brand gold accent on active settings toggle switches (`.abs-slider`) with a standard emerald green (`#10b981`) to match the original Audiobookshelf styling.
  - Aligned troubleshooting / cache tool buttons to use premium dark grey layouts (`bg-black-400 hover:bg-black-300 border border-black-300`) instead of almost-black colors.
  - Refactored the settings sidebar layout to support swiping on mobile view by laying out buttons horizontally with hidden scrollbars, while preserving the vertical sidebar on desktop screens.
  - Grouped sidebar buttons into Server, Configuration, and Tools sections, and implemented dynamic active tab highlights with left gold borders on desktop (`md:border-l-4 md:border-l-accent`) and bottom gold borders on mobile (`border-b-2 border-b-accent`).
  - Added a thick left gold border highlight (`border-l-4 border-l-accent`) to selected library rows inside the libraries settings sub-tab.
  - Implemented gold focus ring glow transitions (`box-shadow: 0 0 0 2px rgba(229, 169, 59, 0.25)`) for settings form input fields, selects, and textareas on focus.
- **Item Details Playback History & Logs Filters**:
  - Integrated the backend `/api/me/listening-sessions?itemId=...` API with `frontend/js/itemDetails.js` to render a "Recent Sessions" panel under the details-rss-section, displaying local formatted dates, durations, playback devices, and methods.
  - Implemented a custom `progress-updated` event framework to update details badges (Finished, In Progress, Not Started), progress bar width, percentage text, and remaining durations dynamically.
  - Added a matching event listener in `createCard` inside `frontend/js/dashboard.js` to update cards in-place upon receiving the `progress-updated` event.
  - Optimized the WebSocket `user_item_progress_updated` handler in `frontend/js/socket.js` to cache updates and fire the local event, reloading the dashboard shelves only if shelf membership changed.
  - Enhanced the "Listening Sessions" logs tab in `frontend/js/settings/logs.js` with comprehensive client-side text search and play method selection dropdown filtering.
- **Visual Parity and Scrollbar Refinements**:
  - Removed `no-scroll` styling from the Playback History (`history-controls`) list, Chapters list (`chapters-list-container`), Podcast Episode lists (`podcast-episodes-list`), and EPUB/PDF E-Reader Sidebars (table of contents, bookmarks, highlights, and thumbnails containers) to allow floating macOS-style scrollbars.
- **Codebase-wide Toast Notification Migration**:
  - Replaced all legacy browser `alert()` modal calls across the entire frontend javascript codebase with modern, custom `showToast()` notifications.
  - Exposed `showToast` globally to the `window` object in `frontend/js/toast.js` and added support for a `'warning'` state alongside `'success'` and `'error'` states.
  - Successfully migrated 30+ alert instances across `collections.js`, `dashboard.js`, `itemDetails.js`, `bookmarksModal.js`, `chaptersModal.js`, `coverEditorModal.js`, `editDetailsModal.js`, `matchBookModal.js`, `playlistModal.js`, `shareModal.js`, `player.js`, `player/ui.js`, `playlists.js`, `presets.js`, `reader.js`, and `reader/bookmarks.js`.
- **Toggle Switch Color Alignment**:
  - Re-aligned the custom checked switch checkbox color (`input:checked + .abs-slider`) in `frontend/css/components.css` to use the standard emerald green (`#10b981`), maintaining consistent visual design and branding.
- **Stats Dashboard & Card / Layout Auditing**:
  - Upgraded the "Recent Playback Sessions" dashboard list in `stats.js` to premium card-like components styling featuring dynamic device icons, play method badges (HLS vs Direct Play), and formatted dates.
  - Aligned "All Playback Sessions" log table in Server Stats tab to render device icons and styled play method badges with appropriate color tokens.
  - Implemented thorough HTML escaping for stats dashboard metadata (genres, authors, titles, usernames) to prevent script injection or broken layouts.
  - Added smooth card lift elevations (`hover:-translate-y-1 hover:shadow-lg`) and duration transition properties to playlist cards (`playlists.js`) and collection cards (`collections.js`), finishing with gold-accented detail links.

### 2026-07-17
- **Library Configuration Selection Highlighting**:
  - Implemented dynamic gold border outlines for selected library rows, unselected library rows hover highlights, and active drag-and-drop event targets inside the Libraries settings sub-tab in `frontend/js/settings.js`.
- **Toggle Switch Restored to Green**:
  - Updated settings toggle switch checked state background color to `#10b981` (emerald green) in `frontend/css/components.css` to match the green/gray look and feel of the original Audiobookshelf settings panels.
- **Responsive Layout Audits & Toolbar Enhancements**:
  - Added full responsiveness constraints to the primary Home/Dashboard header toolbar in [index.html](file:///home/jay/projects/audiobookshelf-go/frontend/index.html), allowing layout elements to wrap dynamically and hiding verbose button text (`Batch Edit`, `OPML`, `Save View`) on mobile screen sizes while retaining the icon.
  - Refactored the Libraries settings tab header layout in [settings.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/settings.js) to wrap description text and stack actions vertically on small viewports.
- **Media Player Responsiveness Auditing & Enhancements**:
  - Aligned sticky bottom `#player-bar` padding to be responsive (`px-4` on mobile, scaling up to `px-6` on tablet/desktop viewports).
  - Redesigned the full-screen expanded player dialog (`triggerExpandedPlayer`) in [ui.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/player/ui.js) to be fully responsive for vertical height constraints and narrow screen widths:
    - Set the cover art image container to scale dynamically based on viewport height (`max-h-[30vh] max-w-[30vh] aspect-square`) to prevent vertical layout overflow.
    - Switched dialog padding, container margins, and inner layout spacing from static large spacing to responsive sizing (`p-4 sm:p-6`, `py-4 sm:py-6`, `space-y-4 sm:space-y-6`).
    - Adjusted font sizes (`text-lg sm:text-xl` for titles, `text-xs sm:text-sm` for author, `text-[10px] sm:text-xs` for chapters) and icon sizes (`text-base sm:text-lg` for secondary buttons) to remain perfectly proportional on mobile.
- **Dynamic Bookshelf & Card Reflection Scaling (Regression Audit)**:
  - Replaced hardcoded `20px` values for wood shelf planks with a dynamic CSS variable `--bookshelf-plank-height` calculated directly from the card width.
  - Calculated `--bookshelf-row-height` dynamically as a function of card height, plank height, and padding bounds, allowing home shelf rows to scale seamlessly without overflow.
  - Fixed cover reflection clipping under `.bookshelfRow` and `.library-shelf-grid` by dynamically terminating the reflection gradient at `--bookshelf-plank-height` and setting the row container's `padding-bottom` to match.
  - Polished the floating `#shelf-size-slider` range input to render with a custom track and a gold/orange thumb that highlights/glows on hover using brand accent colors.




