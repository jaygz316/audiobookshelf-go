# Target Go Package Layout

This document details the target Go package structure and shows how the legacy Node.js source modules (located under `/server`) map onto the Go architecture.

## Layout Overview

All target packages reside within the standard Go module layout. Existing files in the repository root are part of the `main` package and are already partially or fully implemented. New packages are placed under the `internal/` directory to prevent external exposure and ensure clean boundaries.

```
/ (root package main)
├── auth.go                  - JWT structures, AuthMiddleware (already done)
├── backups.go               - Backup endpoints & logic (already done)
├── db.go                    - SQLite connectivity & models (already done)
├── hls.go                   - HLS Streaming (already done)
├── main.go                  - Entrypoint, HTTP route setup (already done)
├── me.go                    - User progress & bookmarks (already done)
├── misc.go                  - Tags, Genres, Stats, Log helpers (already done)
├── scanner.go               - Filesystem library scanner (already done)
├── settings.go              - Server settings & metadata providers endpoints (already done)
├── socket.go                - Socket.io gateway/authority (already done)
├── users.go                 - User CRUD and Login handlers (already done)
├── watcher.go               - FS watcher for library folders (already done)
├── go.mod                   - Module definition: module audiobookshelf (already done)
│
└── internal/
    ├── auth/                - OIDC authentication strategy (new)
    ├── metadata/            - EPUB & Comic metadata extraction helpers (new)
    ├── providers/           - External API clients (Audible, iTunes, Google, etc.) (new)
    ├── finders/             - Higher-level search coordinators (new)
    ├── podcast/             - Subscriptions, schedule checks, episode downloads (new)
    ├── feed/                - RSS / OPML XML feed generation (new)
    ├── playlist/            - Playlists and book collections (new)
    ├── share/               - Public share link generation & access validation (new)
    └── notification/        - SMTP Email, Webhooks, Apprise integration (new)
```

## Module-to-Package Mapping

| Source Node.js Module | Path | Target Go Package | Port Status | Notes |
| :--- | :--- | :--- | :--- | :--- |
| `go-root` | `.` | `main` | `verified` | Contains existing Go files in root. |
| `root-node` | `.` | `main` | `verified` | Main server launch and config parsing. |
| `server-auth` | `server/auth` | `internal/auth` | `pending` | Port OIDC strategy and token verification. (JWT/Local is already in `main`). |
| `server-controllers` | `server/controllers` | Split across packages | `pending` / `verified` | LibraryItem/Me/User/Backup/Settings are `verified` in `main`. Playlist/RSSFeed/Share/Podcast/Notification are `pending` in respective `internal/` packages. |
| `server-core` | `server/` | `main` | `verified` | Core server, database, socket authority, and fs watcher. |
| `server-finders` | `server/finders` | `internal/finders` | `pending` | Coordinator for metadata lookup. |
| `server-libs` | `server/libs` | External dependencies / `main` | `verified` | Vendored npm libraries (bcrypt, jsonwebtoken, etc.) are replaced with idiomatic Go libraries. |
| `server-managers` | `server/managers` | Split across packages | `pending` / `verified` | Cron/Log/Watcher are in `main`. PodcastManager in `internal/podcast`. Notification/Email in `internal/notification`. |
| `server-migrations` | `server/migrations` | `main` (implicit) | `verified` | Schema migrations are handled by the SQLite initializers in `db.go`. |
| `server-models` | `server/models` | Split across packages | `pending` / `verified` | core tables (User, Library, Book) are in `main` (`db.go`). New structures mapped to `internal/playlist`, `internal/podcast`, `internal/share`. |
| `server-objects` | `server/objects` | `internal/podcast` / `main` | `pending` / `verified` | TrackProgressMonitor/PlaybackSession mapped to `internal/podcast`. |
| `server-providers` | `server/providers` | `internal/providers` | `pending` | External search providers (Audible, OpenLibrary, Audnexus, iTunes). |
| `server-routers` | `server/routers` | `main` | `verified` | Express router pathways mapped to setupHandler in `main.go`. |
| `server-scanner` | `server/scanner` | `main` | `verified` | Walk folder logic mapped to `scanner.go`. |
| `server-utils` | `server/utils` | `internal/metadata` / `main` | `pending` / `verified` | Comic/EPUB zip extractors mapped to `internal/metadata`. Regex, file utilities, date helpers are standard Go library uses in `main`. |
