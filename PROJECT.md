# Project: Audiobookshelf Go Refactoring

## Architecture
- **Backend**: Go (root-level `.go` files + `internal/` packages)
- **Frontend**: Nuxt.js/Vue.js (embedded as static assets in `frontend/`)
- **Database**: SQLite with migrations in `internal/db/`

No architectural or interface boundaries should change during this refactoring. All refactored packages must keep their existing public interfaces, APIs, and functions intact. Files are split purely internally within packages.

## Code Layout
- Root packages/commands: `main.go`, `run.go`
- Front-end command/wrapper: `frontend/go/main.go`
- Internal modules: `internal/`
  - `utils/` - Utility functions
  - `logger/` - Thread-safe logging with output buffering
  - `finders/` - Content search/matching helpers
  - `share/` - Share link generation and tracking
  - `auth/` - Authentication logic
  - `notification/` - Push notifications
  - `watcher/` - File system watcher
  - `metadata/` - Metadata extractors (e.g. epub, comic)
  - `providers/` - Metadata providers (audnexus, fantlab, openlibrary)
  - `hls/` - FFmpeg-based transcoding and HLS streaming
  - `db/` - Database schemas, migrations, users/libraries tables
  - `scanner/` - Media scanner and metadata parsers
  - `socket/` - Realtime communication handlers
  - `backup/` - Server backups
  - `feed/` - RSS/Atom feed generation
  - `playlist/` - Playlists CRUD
  - `podcast/` - Podcasts CRUD and queue
  - `handlers/` - API route handlers, middleware, request validation

## Milestones

| # | Name | Scope | Dependencies | Status |
|---|------|-------|--------------|--------|
| 1 | M1: Utilities & Infrastructure | `internal/utils`, `internal/logger`, `internal/finders`, `internal/share`, `internal/auth`, `internal/notification`, `internal/watcher`, root files | none | DONE |
| 2 | M2: Metadata, Providers & HLS | `internal/metadata`, `internal/providers`, `internal/hls` | none | DONE |
| 3 | M3: DB & Scanner | `internal/db`, `internal/scanner` | M1 | DONE |
| 4 | M4: Socket, Backup, Feed, Playlist & Podcast | `internal/socket`, `internal/backup`, `internal/feed`, `internal/playlist`, `internal/podcast` | M1, M3 | DONE |
| 5 | M5: Handlers - Library, Audio, & Sync | `internal/handlers` (Part 1) | M1, M2, M3, M4 | DONE |
| 6 | M6: Handlers - Routes & Rest | `internal/handlers` (Part 2) | M5 | DONE |

## Interface Contracts
All public interfaces must be kept identical. Private helper functions, structs, and methods can be extracted to separate `.go` files in the same packages. No imports or external package functions should be broken.
