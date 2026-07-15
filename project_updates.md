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


