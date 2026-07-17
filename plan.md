# Plan: Audiobookshelf Frontend Parity Enhancements

We are implementing the following visual and interactive refinements to achieve absolute parity with the original Audiobookshelf project:

## 1. Sidebar Link Tooltips & Accessibility
- Add descriptive `title` attributes to all sidetrack/sidebar navigation links (Home, Library, Latest, Series, Collections, Playlists, Authors, Narrators, Stats, Add, Download Queue, Issues).
- Ensure consistent hover title overlays matching the side-rail behavior of the original project.

## 2. Details Page Header Polishing
- Re-align detail view action buttons (Match, Edit Details, Delete) in [itemDetails.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/itemDetails.js) to utilize the premium dark grey layout (`bg-black-400 hover:bg-black-300 border border-black-300`) established during previous audits.
- Refine the detail navigation header's layout, spacing, and back button transition/focus states.

## 3. Top AppBar Notification Tasks Widget Animations
- Clean up and polish the notifications badge ping animation (`#header-notification-badge-ping`) to ensure it scales and pulses smoothly.
- Ensure the spinning sync animation for active tasks in [app.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/app.js) has consistent timing and transitions.

## 4. Compilation & Verification
- Compile the WebAssembly frontend and main Go binary via `run.go` task runner.
- Run integration tests to verify no regressions were introduced.
