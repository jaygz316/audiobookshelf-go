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

### 2026-07-21
- **Login WebAssembly & WebSocket Authentication Verification Fix**:
  - Fixed WASM panic during login form submission by adding safe DOM type checks (`js.TypeNull` and `js.TypeUndefined`) across [login.go](file:///home/jay/projects/audiobookshelf-go/frontend/go/login.go), [auth.go](file:///home/jay/projects/audiobookshelf-go/frontend/go/auth.go), and [setup.go](file:///home/jay/projects/audiobookshelf-go/frontend/go/setup.go).
  - Resolved session verification error in `AuthMiddleware` ([middleware_auth.go](file:///home/jay/projects/audiobookshelf-go/internal/handlers/middleware_auth.go)) where API requests after login received 401 Unauthorized status and triggered frontend `auth-unauthorized` logout handler.
  - Configured Engine.io `opts.SetPath("/socket.io")` and `opts.SetAllowEIO3(true)` in [events.go](file:///home/jay/projects/audiobookshelf-go/internal/socket/events.go) and Engine.io polling session handshake in [socket.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/socket.js).
  - Bumped frontend ServiceWorker cache to `v4` in [sw.js](file:///home/jay/projects/audiobookshelf-go/frontend/sw.js) and [index.html](file:///home/jay/projects/audiobookshelf-go/frontend/index.html) using a Network-First strategy for application assets.
  - Built and pushed updated Docker Hub container image `jaygz/audiobookshelf-go:latest`.

### 2026-07-17
- **Player Speed and Premium Modals UI Polish**:
  - Dynamically populated all speed dropdown selectors (`#player-speed`, `#expanded-speed`, and `#speed-default-select` in the settings dialog) to support fine-grained playback speed controls from 0.5x to 3.0x in 0.05x increments.
  - Converted checkboxes in **Player Settings Modal** (`#speed-remember-input`) and **Sleep Timer Modal** (`#sleep-autorestart-input`, `#sleep-shaketoreset-input`) into premium sliding switches (`.abs-switch`), standardizing input elements and settings toggles to match design system guidelines.
- **Upload Modal Target Library & Config Standardizations**:
  - Implemented dynamic target library switching inside both the files upload modal and the podcast subscription modal in [upload.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/upload.js), allowing users to switch libraries seamlessly.
  - Standardized target configuration elements in both modals inside structured grid cards, matching project-standard theme-aware input controls.
  - Saved folders details dynamically upon target library change, switching display modes if changing between book and podcast library types.
- **Upload Page Scrollbars & Background Opacity**:
  - Replaced the non-existent class `scrollbar-thin` with standard `no-scroll` in [upload.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/upload.js) to ensure consistent, elegant scrollbars on the Upload media modal.
  - Added missing transparent background utility classes `.bg-black-500/20`, `.bg-black-500/30`, `.bg-black-500/40`, `.bg-black-500/60`, and `.bg-black-500/70` in [components.css](file:///home/jay/projects/audiobookshelf-go/frontend/css/components.css) to support theme-aware background rendering.
- **Podcast Cover Aspect Ratios & Details Styling**:
  - Introduced `.podcast-library` class to dynamically override bookshelf card height (`--bookshelf-card-height: var(--bookshelf-card-width) !important`) and recalculate row heights inside `layout.css`.
  - Configured `loadDashboard` in `dashboard.js` to automatically toggle `.podcast-library` class on the bookshelf container.
  - Refactored `itemDetails.js` to size podcast covers as `w-56 h-56` square and conditionally render the book spine crease overlay (`.book-spine-crease`) only for non-podcast items.
  - Adjusted `coverEditorModal.js` to preview search result images with square or rectangular aspect ratio classes according to the media type.
- **Library Selection Color Alignment**:
  - Aligned the active settings library selection highlights with the authentic orange theme accent (`#e88024`) instead of default gold/theme accent classes.
- **Interactive Modals & Metadata Provider Refinements**:
  - Converted checkboxes inside [editDetailsModal.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/modals/editDetailsModal.js) and [bookmarksModal.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/modals/bookmarksModal.js) to the premium sliding switch component (`.abs-switch`), standardizing configuration toggles across all user input forms.
  - Enabled metadata and cover search provider selection for podcast libraries inside [matchBookModal.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/modals/matchBookModal.js), resolving providers to podcasts-specific listings (iTunes) when handling podcast items.
  - Aligned search filter categories list in [app.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/app.js) to dynamically label the "Author" filter option to "Publisher" when filtering a podcast library.
  - Added the bookshelf wooden divider plank under category placards in the shelf grid sections (`createShelfGridSection` in [dashboard.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/dashboard.js)), aligning grid shelf styles with horizontal row shelf styles.
- **Settings Modals Scroll Containment Refactoring**:
  - Standardized scroll containment across `showLibraryModal`, `triggerCreateNotificationModal`, and `triggerEreaderDeviceModal` in [settings.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/settings.js).
  - Replaced full viewport scrolling overlay layouts with capped flex overlays (`max-h-[90vh] flex flex-col`) and nested scrollable field containers (`.flex-grow.overflow-y-auto.no-scroll.pr-1`), ensuring input forms are perfectly constrained on narrow devices and do not bleed below the viewport fold.
- **Settings Tabs Navigation & Visual Auditing**:
  - Standardized list action buttons (Delete, Edit, Copy, Close Feed) across all settings sub-panes (including Backups, RSS Feeds, Devices/Sessions, API Keys, and Apprise setups) in [settings.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/settings.js) and [backups.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/settings/backups.js) to use premium colored backgrounds and borders matching dark theme warning/success variable presets.
  - Refactored authentication method configuration checkboxes to use responsive stacked-to-row alignments (`flex-col sm:flex-row`) preventing text wrap clipping on mobile.
  - Verified back-button page/state synchronization across browser navigation, scroll alignments inside tab rails, and lifecycle tear-downs.
- **Settings & Share Visual Parity Enhancements**:
  - Upgraded the "Active Metadata Providers" list in [settings.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/settings.js) to display book and podcast providers as clean, unified badges with appropriate theme-aware icons.
  - Enhanced the "Active Share Links" list in [settings.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/settings.js) to show a rich item preview column with cover thumbnails, media titles, type labels/icons, custom status pills for protection/embedding configurations, and polished warning-styled revoke buttons.
  - Polished the delete buttons in system notifications setups in [settings.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/settings.js) to match the dark theme warnings styling.
  - Converted share link creation configuration options in [shareModal.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/modals/shareModal.js) from checkboxes to premium sliding switches (`.abs-switch`).
- **Tasks & Downloads General Tasks Rendering**:
  - Refactored `updateTasksList` in [logs.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/settings/logs.js) to display general background tasks correctly. If standard podcast metadata fields (`podcastTitle` or `episodeTitle`) are empty, cells fall back to displaying the task `name`, `type`, and `description`.
- **Nested Directory Scanning Resilience**:
  - Implemented `TestScanNestedDirectoriesResilience` in [scanner_test.go](file:///home/jay/projects/audiobookshelf-go/internal/scanner/scanner_test.go) to verify paths with deep nested hierarchies, ensuring that directory scanning does not suffer from regressions.
- **Theme-Aware Bookshelf Back Wall Overlays**:
  - Replaced hardcoded dark background overlay values (`rgba(20, 20, 20, 0.75)` and `rgba(20, 20, 20, 0.55)`) on `.bookshelfRow` and `.library-shelf-grid` with theme-aware dynamic variables (`--bookshelf-wall-overlay-top` and `--bookshelf-wall-overlay-bottom`).
  - Defined custom wall overlay values for light, sepia, and dark themes inside [variables.css](file:///home/jay/projects/audiobookshelf-go/frontend/css/variables.css), ensuring wood texture styling matches surrounding background contexts seamlessly.
- **Library Grid, Header Navbar Search & Chromecast Alignment**:
  - Implemented dynamic update of the global search input placeholder to reflect the active library's media type (e.g. `"Search Books..."` or `"Search Podcasts..."`), resolving dynamic viewport placeholders.
  - Hidden secondary/administrative header actions (`#header-settings-btn`, `#header-activity-btn`, and `#header-upload-btn`) on mobile viewports (< 768px) via media query to eliminate crowding and redundancy on narrow screens.
  - Aligned Google Cast connection color state with the application gold accent (`--color-accent` / `#e5a93b`) and corrected sizing for standard alignment.
  - Integrated smooth width, height, and max-width transitions for bookshelf cards to ensure seamless card scaling when adjusting the shelf-size slider.
  - Dynamically toggled `font-semibold` class on active navigation links' label element (`p`) inside `highlightSidebarLink` (`router.js`) to ensure typography weight mirrors the active page context on the navigation rail.
- **Library Layout Columns & Mobile Form Polish**:
  - Refactored [itemDetails.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/itemDetails.js) container padding to responsive `p-4 sm:p-6 md:p-8` and increased maximum width constraint to `max-w-6xl` to align with the main application grid.
  - Upgraded details layout grid on desktop from a hardcoded 3-column split to a customized 2-column layout (`grid-cols-1 md:grid-cols-[240px_1fr]`) to reserve a precise 240px left-hand control rail.
  - Configured core left-rail widgets (Play/Read buttons block, progress summary, RSS status cards, playback history block) to expand dynamically to fill the column width on desktop (`md:max-w-none`) while remaining compact (`max-w-xs`) and centered on mobile viewports.
  - Aligned [authors.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/authors.js) author and series details views with standard details page responsive margins (`p-4 sm:p-6 md:p-8 max-w-6xl`).
  - Refactored `triggerUserModal` in [users.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/settings/users.js) to enforce scroll containment on small screens (using a standard flex layout container with `max-h-[90vh] flex flex-col` and a nested scrollable field wrapper `.flex-grow.overflow-y-auto.no-scroll.pr-1`).
  - Added a clean top divider border and kept actions footer fixed visually at the bottom of the modal, ensuring user forms do not bleed past the viewport bottom on mobile.
- **Mobile Dropdown Drill-Down & Accessibility Audit**:
  - Enhanced the filter submenu UX in [app.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/app.js) on mobile viewports by hiding the parent categories filter dropdown menu when the submenu is active. Clicking "Back to Categories" within the submenu restores the parent menu visibility. This mimics native navigation stacks and avoids overlapping semi-transparent glassmorphic overlays.
  - Increased the touch target size of metadata lock buttons (`metadata-lock-btn`) in [editDetailsModal.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/modals/editDetailsModal.js) by adding padding (`p-2`), standard sizing (`w-8 h-8 flex items-center justify-center`), and hover/focus highlights within a rounded-full layout.
  - Modified the lock click binding in [editDetailsModal.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/modals/editDetailsModal.js) to toggle specific color classes using `classList` rather than overriding the entire `className`, preserving the custom layout styles and preventing tap target collapse on interaction.
  - Configured long-text secondary buttons (Embed Metadata, Merge Audio Files, and Share Link) on the item details page in [itemDetails.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/itemDetails.js) to span the full 2 columns (`col-span-2 sm:col-span-1`) on mobile viewports. This prevents narrow text wrapping and ensures perfectly balanced button rows with no orphaned cells.
- **Mobile Details Action Button Grid & Toolbar Dropdown Enhancements**:
  - Refactored the item details page action buttons list in [itemDetails.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/itemDetails.js) to display as a clean 2-column grid (`grid grid-cols-2 gap-2`) on mobile viewports with primary play and read buttons spanning both columns, replacing a long vertical stack of buttons and removing hardcoded top margins.
  - Optimized the filter submenu flyout positioning in [app.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/app.js) to overlay on top of the main filter category menu on mobile viewports (`right: 0px`, `top: 100%`, `width: 192px`), and added a sticky "Back to Categories" header action button to return to the category selection drawer.
  - Increased the width of main filter and sort dropdowns in [index.html](file:///home/jay/projects/audiobookshelf-go/frontend/index.html) on narrow screens (`w-48 sm:w-44`) to enlarge touch targets and minimize title text truncation.
- **Modal Text Selection and Inputs Fix**:
  - Removed `select-none` from the modal wrapper divs across all dynamic modals (`bookmarksModal.js`, `editDetailsModal.js`, `matchBookModal.js`, `shareModal.js`, `chaptersModal.js`, `coverEditorModal.js`) to allow text highlight, select, and copy/paste within forms.
  - Added targeted `select-none` wrappers specifically to crop canvas and timelines inside those modals (e.g. cover editor canvas container) to prevent accidental text highlight during drag interactions.
- **Podcast Episodes List Mobile Layout Audit**:
  - Implemented responsive wrapping and text truncation on individual episode buttons (Play, Resume, Replay, Download) by hiding button text (`hidden sm:inline`) on narrow viewports while maintaining descriptive titles for screen readers.
  - Wrapped batch actions toolbar in flex-wrap and adjusted button layout to prevent viewport overflow on small screens.
- **Smooth Details Transitions**:
  - Added smooth CSS Grid transition rules (`grid-template-rows 0.25s ease-out`) on details dropdown elements (`details.group > div`) to enable premium accordion-style expand and collapse animations.
- **Chapters Timeline Touch Optimization**:
  - Implemented touch event listeners (`touchstart`, `touchmove`, `touchend`, `touchcancel`) on the interactive timeline handles in [chaptersModal.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/modals/chaptersModal.js).
  - Added `moveEvent.preventDefault()` inside touch move handlers to prevent default screen panning and scrolling when adjusting chapter boundaries on mobile.
- **Bookshelf and Layout Scroll Physics**:
  - Audited and verified smooth momentum scrolling (`-webkit-overflow-scrolling: touch`) and smooth scroll behavior (`scroll-behavior: smooth`) on all custom horizontal bookshelf rows (`.bookshelfRow`) in [components.css](file:///home/jay/projects/audiobookshelf-go/frontend/css/components.css).
- **Settings Screen Visual Parity**:
  - Verified visual consistency of layout forms, input fields, and green/gray switch toggles (`#10b981` when active).
  - Aligned library lists with left accent orange border (`border-l-accent` / `#e88024` border-left highlight) on selected items, custom styled scan buttons, and active drag handle grab states.
- **Interactive Waveform Chapters Timeline Editor**:
  - Implemented a complete interactive visual waveform timeline for the `Edit Chapters` modal, featuring Zoom controls (1x to 10x), ruler ticks based on duration, and draggable chapter segments to adjust chapter start/end times.
  - Added seamless state synchronization between the chapters list and the visual segments, split/delete actions, ASIN warnings/validations, and loading state indicators.
- **Fullscreen Rotating Circular Cover Art**:
  - Upgraded the cover art in the fullscreen overlay player to be circular (`rounded-full` with a defined border) across all media types.
  - Implemented a smooth spin animation (`animate-spin-slow`) when playback is active, which seamlessly pauses/resumes in place using a toggleable `.animation-paused` class.
- **Modal Input & Native Dialog Visual Standardization**:
  - Implemented theme-aware overrides (`var(--color-black-500)`, `var(--color-black-300)`, and `var(--color-white)`) for inputs, selects, and textareas inside dynamic overlays (`div[class*="fixed"][class*="z-50"]`), settings panel content (`#settings-tab-content`), onboarding wizard (`#setup-screen`), and native `dialog` containers.
  - Standardized transition animations and active focus outlines (accent colors and glow shadows) for form fields to guarantee visual alignment and high-contrast accessibility.
- **Desktop Collapsible Sidebar, Header Branding & State Persistence**:
  - Implemented a premium desktop collapsible sidebar navigation that defaults to a wide layout (`15rem / 240px`) showing text next to icons, and collapses to a compact rail (`5rem / 80px`) showing only icons.
  - Linked the hamburger menu toggle button in the header on desktop viewports to toggle the sidebar's collapse state with smooth CSS transitions.
  - Standardized state persistence by storing the sidebar collapse state in `localStorage` under `sidebar-collapsed` and initializing the layout dynamically on application bootstrap.
  - Styled the header branding title text using white and orange accent variables (`audiobook<span class="text-accent">shelf</span>`) and added a dedicated `#sidebar-branding` block inside the sidebar itself, visible only when expanded.
  - Removed hardcoded inline layout widths from the HTML to allow pure, responsive CSS control.
- **Cascading Fanned Series Cover Cards with 3D Spine Creases & Reflections**:
  - Wrapped stacked series covers on both the series list card and details view in `.series-cover-book.book-cover-wrapper` containers.
  - Injected `.book-spine-crease` overlays onto stacked covers to mimic realistic 3D book cover depth.
  - Styled `.series-cover-front` in [components.css](file:///home/jay/projects/audiobookshelf-go/frontend/css/components.css) to support percentage-based reflections (`-webkit-box-reflect: var(--bookshelf-reflect)`), aligning series cover styling with individual bookshelf card views.
- **3D Bookshelf Side Borders, Theme-Aware Planks & Premium Sizing Slider**:
  - Implemented left and right vertical wooden side panels on `.library-shelf-grid` and `.shelf-wrapper` (Home page scrolling rows) to complete the premium wooden bookshelf aesthetic across all views and themes.
  - Standardized the `--bookshelf-texture-img` variable definition directly inside [variables.css](file:///home/jay/projects/audiobookshelf-go/frontend/css/variables.css) to guarantee its availability across all stylesheets.
  - Integrated theme-aware overlays (`--bookshelf-overlay`) into `.bookshelfDividerCategorized` backgrounds, ensuring sepia and light themes dynamically tint the planks correctly.
  - Custom-styled the `#shelf-size-slider` runnable track and thumb with dedicated hover scale effects, consistent borders, and cross-browser (Webkit/Firefox) support.
  - Defined explicit `:root[data-theme="dark"]` stylesheet variables in [variables.css](file:///home/jay/projects/audiobookshelf-go/frontend/css/variables.css) to guarantee clean, robust variables overriding during theme toggling.
- **Verification of Timeline Bar Chapters & Cover UX**:
  - Performed a visual audit on the player timeline chapter marks (tick-marks and tooltip titles) and book cover overlays (spine crease, shadow elevation, and reflections), confirming complete parity and functional integration with the WebAssembly and API backends.
  - Verified compilation and build compatibility using the unified Go task runner build parameters (`go run run.go run_commands.go build` and `test`), ensuring all backend test suites pass with 100% compliance.
- **Smooth SPA View & Settings Tab Transitions**:
  - Implemented the modern CSS View Transitions API in [router.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/router.js) and [settings.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/settings.js), wrapping page switching and Settings tab navigation view updates. This replaces abrupt route and tab jumps with beautiful, hardware-accelerated transitions that match the premium visual standards of Vue.js frameworks.
- **Library Dropdown Polishing and Active Highlighting**:
  - Styled the library dropdown button in [index.html](file:///home/jay/projects/audiobookshelf-go/frontend/index.html) to perfectly match the user menu button style (`bg-black-600 border border-black-400/50 hover:bg-black-500 rounded px-2.5 h-8 flex items-center text-xs font-semibold text-white/90`).
  - Added active highlighting and checkmark support to the library dropdown items in [library.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/library.js), formatting the selected library with a gold checkmark icon and highlighted accent styles (`text-accent font-semibold bg-black-400/20`).
- **Search Suggestions & Filter Submenu Placement Audit**:
  - Cleaned up inline Tailwind background and border styles from search dropdown section headers and items in [app.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/app.js) to allow project-standard CSS variables and hover/active states to apply cleanly without overrides. Simplified selection highlighting logic.
  - Implemented dynamic submenu horizontal positioning in [app.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/app.js) to detect available viewport space and overlay categories or shift position on narrow devices, avoiding horizontal off-screen drawer overflows.
- **Login and Onboarding Setup Wizard Theme Standardization**:
  - Replaced legacy Tailwind gray text and border classes on the login page and initial setup onboarding wizard screens in [index.html](file:///home/jay/projects/audiobookshelf-go/frontend/index.html) with standardized, premium semantic variables (`text-black-50`, `text-black-100`, `border-black-400/50`, and custom focus border states).
- **Global Interactive Button Cursors**:
  - Injected global CSS cursor rules inside [base.css](file:///home/jay/projects/audiobookshelf-go/frontend/css/base.css) to apply pointer indicators (`cursor: pointer`) to all `<button>` and `[role="button"]` elements, and a `not-allowed` pointer state to all disabled buttons. This ensures that every interactive button, modal action, and dropdown trigger across the entire application consistently displays the standard hand cursor.
- **User and Settings Management Action Controls Styling**:
  - Upgraded unlink OIDC, edit, and delete action buttons inside [users.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/settings/users.js) to utilize standard warning (`bg-warning/20 text-warning border-warning/30`) and error (`bg-red-900/40 text-error border-red-500/30`) theme variables instead of legacy Tailwind colors.
  - Aligned API key delete action buttons to use warning/error color variables.
- **Interactive Bookshelf Row Scroll Buttons**:
  - Added left/right scroll buttons on personalized shelves ("Continue Listening", "Recently Added", etc.) with auto-hidden states on overflow start/end boundaries, complete with smooth scroll triggers and premium CSS hover/active effects, bringing complete visual and functional parity to horizontal shelf navigation.
- **Settings Form Segmented Controls & Custom Dialogs**:
  - Replaced native select inputs with green/gray CSS pill-segmented controls for "Tag Filter Mode", "Media Type", and "Cover Aspect Ratio" across user/library/provider modals to mirror Vue-like toggles.
  - Added a globally accessible `showConfirm` dialog in `toast.js` attached to `window.showConfirm` to replace standard browser `confirm()` calls with stylized dark charcoal/gold border overlays.
  - Corrected mobile view responsiveness of Settings tab layout by adjusting padding size variables on smaller screen sizes.
- **Responsive Mobile Navigation Drawer**:
  - Redesigned the mobile navigation sidebar drawer (`#sidebar`) to be a widescreen (16rem / 256px wide) slide-out panel matching standard native drawer menus.
  - Formatted sidebar navigation link items horizontally on mobile viewports, displaying the page icon and text label side-by-side with improved text sizes and alignment.
  - Adjusted the mobile sidebar drawer's footer (help, version, and source buttons) to layout as a clean horizontal flex row with subtle styling.
- **Details Page Header & Action Controls Styling**:
  - Updated the Back button, administrator action buttons (Match, Edit Details, Delete), and all core action buttons (Queue, Send to Device, Playlist, Download, cover/metadata updates, etc.) on the item detail view in [itemDetails.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/itemDetails.js) to utilize consistent, premium dark grey colors and layouts (`bg-black-400 hover:bg-black-300 border border-black-300`).
- **Sidebar Navigation Accessibility & Tooltips**:
  - Added descriptive `title` attributes to all sidetrack/sidebar navigation link buttons in [index.html](file:///home/jay/projects/audiobookshelf-go/frontend/index.html) to support standard responsive side-rail hover tooltips.
- **Search & Sort Toolbar Controls Polishing**:
  - Styled search and sort controls in Authors, Series, and Narrators views in [authors.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/authors.js) and [narrators.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/narrators.js) with premium dark grey selectors and inputs (`bg-black-400 border border-black-300`).

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
- **Library Grid Card Sizing & Main View Transition Animations**:
  - Standardized card sizing logic globally in standard libraries grid views by defining `.library-grid .bookshelf-card` styling rules in [layout.css](file:///home/jay/projects/audiobookshelf-go/frontend/css/layout.css), ensuring book cards automatically scale with the active shelf-size slider across all grid pages (including authors, playlists, and collection detail views).
  - Implemented high-performance View Transitions support and smooth fallback cross-fade transitions (`#bookshelf > div`) for the main content area in [components.css](file:///home/jay/projects/audiobookshelf-go/frontend/css/components.css) to achieve fluid view switches when navigating between pages.
  - Enabled smooth scrolling and mobile touch momentum scrolling (`-webkit-overflow-scrolling: touch`) on horizontal bookshelf rows (`.bookshelfRow`) in [components.css](file:///home/jay/projects/audiobookshelf-go/frontend/css/components.css) to optimize touch responsiveness on iOS and Android devices.
- **Typography, Font Parity & Header Spacing Polish**:
  - Embedded Google Fonts `Source Sans Pro` link globally in [index.html](file:///home/jay/projects/audiobookshelf-go/frontend/index.html) and applied it as the primary font-family for `body` in [base.css](file:///home/jay/projects/audiobookshelf-go/frontend/css/base.css) to achieve complete typography parity with the original Audiobookshelf project.
  - Adjusted top header brand logo and title (`audiobookshelf`) sizes, weights, and tracking (`text-lg font-semibold tracking-wide`) to match original brand specs.
  - Expanded the top header search bar (`global-search-container`) maximum width to `max-w-md` and adjusted spacing (`mx-8` margin) on desktop viewports to match original layout proportions.
  - Performed a responsive layout audit of the Series list and Series detail pages, ensuring cover stacks (`series-detail-cover-stack`) and versions grid containers behave fluidly on both desktop and mobile viewports.
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

- **Unified Details Views Back Navigation & Action Styling**:
  - Aligned detail views back navigation links (Series, Collections, Playlists, Authors) to use high-contrast `text-black-100` and hover `text-white` with `transition-colors cursor-pointer` and Material Symbols back arrow icons.
  - Replaced legacy color styling of administrative action buttons (Edit, Match, Auto-Number, Play, Delete) on Series, Collections, and Playlists detail screens with premium dark-grey variables (`bg-black-400 hover:bg-black-300 border border-black-300 text-white font-semibold rounded text-xs flex items-center space-x-1 transition-colors cursor-pointer`).
  - Styled delete confirmation buttons with red border details and low opacity red background highlights (`bg-black-400 hover:bg-red-900/40 border border-red-500/30 text-error hover:text-white hover:border-red-500/50`) matching the main item details view delete buttons.
  - Added clean inline Material Symbols icons (`edit`, `delete`, `find_replace`, `play_arrow`, `format_list_numbered`) to detail action controls for a premium look and feel.
  - Aligned Podcast Download Queue cancel/pause/resume buttons with identical premium button classes and added `cursor-pointer` to episode play/download items.

- **Settings Tabs & Color Token Parity Audit**:
  - Audited all settings tab forms (Server, Auth, Notifications, Providers, Email, and Users settings) for design system compliance and color token purity.
  - Replaced legacy Tailwind red/green/yellow/blue text and background color classes (`text-red-400`, `text-red-500`, `text-green-400`, `text-yellow-400`, `bg-red-500`, etc.) with custom theme variables (`text-error`, `text-success`, `text-warning`, `text-info`) across all settings screens, user modals, backup list rows, active tasks list rows, and logging console rows.
  - Refined settings active task list status badges (Downloading, Paused, Failed, Completed) to use consistent, premium border-badge styles (e.g., `bg-info/10 text-info border border-info/30`, `bg-success/10 text-success border border-success/30`).
  - Rebuilt and verified full Go backend and WebAssembly integration and test integrity successfully.

- **Drag-and-Drop Constraints & Search Dropdown Category Count**:
  - Constrained drag-and-drop initiation for table/list row reordering (settings libraries, collections, playlists, and player queue) to trigger only on the `.drag-handle` element (by checking `e.target.closest('.drag-handle')` in the `dragstart` event). This prevents accidental row dragging and matches the original project's behavior.
  - Refined global search suggestion dropdown categories (Books, Podcasts, Episodes, Authors, Series, Narrators, Tags, Genres) to show result counts in parentheses (e.g., `Books (3)`) matching original Audiobookshelf suggestion behavior.
  - Aligned search dropdown headers to use consistent design-system variables (`text-black-50` and `bg-black-700/60`) for smooth dark theme transitions.

- **Bookshelf View & Card Layout Wood Texture Refinement**:
  - Replaced solid back-wall colors (`var(--color-bg)`) inside `.bookshelfRow` in [components.css](file:///home/jay/projects/audiobookshelf-go/frontend/css/components.css) and `.library-shelf-grid` in [layout.css](file:///home/jay/projects/audiobookshelf-go/frontend/css/layout.css) with a translucent 3D shadow gradient overlay (`rgba(20, 20, 20, 0.75)` to `rgba(20, 20, 20, 0.55)`). This reveals the underlying wood default texture on the bookshelf back wall with realistic depth, matching the premium visual style of the original Audiobookshelf bookshelf view.

- **Responsive Layout Audits & Settings Tables**:
  - Replaced `overflow-hidden` with `overflow-x-auto` in the primary library list view table wrapper (`.library-list-wrapper` in [dashboard.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/dashboard.js)) to guarantee horizontal scroll capabilities on mobile/tablet viewports and prevent table content clipping.
  - Verified that all settings sub-tabs tables (RSS feeds, E-Reader devices, active share links, listening sessions, login sessions, task list rows, backup rows, API keys, and custom metadata providers) are fully wrapped in `.overflow-x-auto` containers to avoid horizontal layout breaks on narrow screens.
  - Audited the Cover Art Canvas Editor modal layout structure in [coverEditorModal.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/modals/coverEditorModal.js) to ensure grid columns collapse smoothly (`grid-cols-1 md:grid-cols-2`) and tab headers support horizontal scrolling (`overflow-x-auto whitespace-nowrap scrollbar-none`).
- **Drag-and-Drop Constraints via Click Origin Verification**:
  - Implemented dynamic `draggable` attribute toggling via a `mousedown` event listener on row elements in [settings.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/settings.js), [collections.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/collections.js), [playlists.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/playlists.js), and [player/queue.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/player/queue.js). This resolves standard browser limitations where the `dragstart` event target is the container row itself, ensuring that drag-and-drop operations only trigger when dragging begins directly on the `.drag-handle`.
- **Podcast Add Icon Conversion**:
  - Converted the legacy `abs-icons` layout inside the podcast "Add" link in the left navigation sidebar in [index.html](file:///home/jay/projects/audiobookshelf-go/frontend/index.html) to a standard, high-readability Material Symbol ligature (`add_circle`), completing the navigation icons normalization audit.
- **Theme-Aware Scrollbar Styling & Visual Parity**:
  - Defined theme-specific scrollbar color variables (`--color-scrollbar` and `--color-scrollbar-hover`) in [variables.css](file:///home/jay/projects/audiobookshelf-go/frontend/css/variables.css) for all themes (light, sepia, dark, and root default).
  - Applied the new variables inside [components.css](file:///home/jay/projects/audiobookshelf-go/frontend/css/components.css) to customize the standard `::-webkit-scrollbar-thumb` and its `:hover` pseudoclass, replacing neutral gray scrollbars with the themed accent color (e.g. golden brown `#855620` on the dark theme) to achieve complete design parity.
- **Initials Fallback Generators & Theme Accent Switch Colors**:
  - Upgraded initials avatar fallback calculations in `app.js` and `authors.js` to split space-separated names (e.g., "John Doe" becomes "JD") while safely falling back to the first two characters for single-word names, achieving complete parity with original profile widgets.
  - Linked active switch checkbox colors in `components.css` to the theme-aware `var(--color-accent)` token rather than hardcoding static colors.

