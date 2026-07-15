# AGENTS.md

This file provides guidance to AI coding assistants and agentic developers when working with code in this repository.

## Crucial: Stale Prevention (Read First)

Before writing any code or proposing modifications, **always read `project_updates.md`** at the root of the repository. This file contains the latest list of deprecated libraries, APIs no longer in use, and active architectural rules. 

If your task deprecates any feature, pattern, or library, or changes project direction (e.g. "no longer using X"), you **must** update `project_updates.md` immediately before concluding your work.

## Project Overview

Audiobookshelf is a Go rewrite of the popular [Audiobookshelf](https://github.com/advplyr/audiobookshelf) server. It provides audiobook and podcast streaming with a Vue.js/Nuxt frontend. The backend is written in pure Go using SQLite (`modernc.org/sqlite` without CGO), HLS streaming with FFmpeg, and real-time Socket.io connections.

## Architecture

### High-Level Structure
- **Backend**: Go (root-level `.go` files + `internal/` packages)
- **Frontend**: Nuxt.js/Vue.js (embedded as static assets in `frontend/`)
- **Database**: SQLite with migrations in `internal/db/`

### Core Backend Packages

**Root-Level Handlers** (main request flows):
- `internal/handlers/`: HTTP route handlers, middleware, and request processing (~800+ lines across multiple files)
  - `routes.go`: Defines all HTTP routes and serves as the request routing hub
  - `middleware.go`: Authentication and authorization middleware
  - `library_handlers.go`: Library CRUD and item management
  - `authors_series.go`: Author and series endpoints
  - `search_handlers.go`: Search functionality
  - `settings_*.go`: Server, auth, and metadata settings endpoints
  - `users.go`: User management and access control
  - `playlist_handlers.go`: Playlist management
  - `backup_handlers.go`: Server backup endpoints
  - `oidc_handlers.go`: OpenID Connect authentication

**Core Data & State** (`internal/core/`):
- `core.go`: Shared types (`UserSession`, `ContextKey`) used across handlers

**Database** (`internal/db/`):
- Schema definitions, migrations, and query builders
- Tables: users, libraries, libraryItems, books, podcasts, series, mediaProgresses, playlists, etc.

**Real-Time Communication** (`internal/socket/`):
- Socket.io namespace handlers for live updates, user presence, library scanning events

**HLS Streaming** (`internal/hls/`):
- FFmpeg-based on-the-fly transcoding and HLS segment serving

**Library Management** (`internal/scanner/`, `internal/watcher/`):
- `scanner/`: Traverses directories, parses audio metadata (ID3, Vorbis), indexes books/podcasts/series
- `watcher/`: Watches library directories for file changes using `fsnotify` and triggers rescans

**Metadata & Providers** (`internal/metadata/`, `internal/providers/`):
- Metadata enrichment from external providers (e.g., Google Books, Audible)

**Additional Features**:
- `internal/backup/`: Database and metadata backups
- `internal/feed/`: RSS feed generation
- `internal/playlist/`: Playlist model and operations
- `internal/auth/`: User authentication and OIDC
- `internal/notification/`: Push notifications
- `internal/share/`: Share link generation and tracking
- `internal/podcast/`: Podcast-specific operations
- `internal/logger/`: Thread-safe logging with output buffering
- `internal/finders/`: Content search/matching helpers
- `internal/utils/`: Utility functions

### Frontend Structure
- `frontend/`: Embedded static assets (compiled by `npm run generate`)
- Built with Nuxt.js/Vue.js and served by the Go HTTP server as an SPA

## Development Commands

### Building

**Build the Go backend** (requires frontend to be built first):
```bash
go build -o audiobookshelf .
```

**Build the frontend** (from `frontend/` directory):
```bash
cd frontend
npm ci          # Install dependencies
npm run generate # Generate static Nuxt build
cd ..
```

**Build with Docker**:
```bash
docker compose up -d --build
```

### Running

**Run the backend** (after building):
```bash
./audiobookshelf --port=13378 --config="./config" --metadata="./metadata"
```

Or with environment variables:
```bash
PORT=13378 CONFIG_PATH="./config" METADATA_PATH="./metadata" ./audiobookshelf
```

**Run via Docker Compose**:
```bash
docker compose up -d
```

### Testing

**Run all tests**:
```bash
go test ./... -v
```

**Run tests in a specific package**:
```bash
go test ./internal/handlers -v
```

**Run a single test**:
```bash
go test -run TestFunctionName ./internal/handlers -v
```

**Run tests with coverage**:
```bash
go test ./... -cover
```

**Run integration tests** (in `e2e/`):
```bash
cd e2e
npm ci
npm test
cd ..
```

### Development Workflow

1. **Backend changes**: Edit `.go` files in root or `internal/`, rebuild with `go build`, restart the server.
2. **Frontend changes**: Edit files in `frontend/`, rebuild with `npm run generate`, restart the server (which serves the new assets).
3. **Database schema changes**: Add migrations in `internal/db/` and follow the migration pattern used in existing files.

## Key Concepts & Patterns

### Request Authentication
- Middleware in `internal/handlers/middleware.go` extracts JWT tokens and loads `UserSession` into request context.
- `UserSession.CanAccessLibrary()` and `UserSession.IsAdminOrUp()` are used to enforce access control.

### Real-Time Updates
- Socket.io handlers in `internal/socket/` broadcast library changes, scan progress, and user presence.
- Progress syncing and media playback events are handled via Socket.io, not HTTP polling.

### HLS Streaming
- FFmpeg transcoding is triggered on-demand by `internal/hls/` handlers.
- Segments are cached in `/metadata/streams/` for reuse.
- Stream sessions are cleaned up hourly if inactive for >36 hours.

### Library Scanning
- `internal/scanner/` recursively traverses library folders, parses audio file metadata, and stores indexed items.
- `internal/watcher/` monitors for file system changes and re-indexes affected items.

### Database
- SQLite is used without CGO (`modernc.org/sqlite`). All tables are defined in `internal/db/`.
- Queries use standard `database/sql` package. Consider using parameterized queries to prevent SQL injection.

### Frontend Integration
- Frontend is embedded in the binary via `//go:embed frontend` in `main.go`.
- SPA fallback routing is handled by the Go server to support client-side routing.

## Important Files

- `main.go`: Server entry point, config parsing, database initialization, HTTP router setup
- `main_test.go`: Test database setup and helper functions
- `internal/handlers/routes.go`: Complete HTTP route definitions (start here for API overview)
- `internal/db/`: All database-related code
- `Dockerfile`: Multi-stage Docker build (frontend + Go binary)
- `docker-compose.yml`: Local development setup with volumes for audiobooks, podcasts, metadata, config

## Testing Strategy

- Tests use in-memory SQLite databases (`:memory:`) for isolation.
- Test fixtures and helpers are in `*_test.go` files adjacent to implementation.
- ~134 test functions across the codebase.
- Use `testify/assert` or Go's built-in `testing` package for assertions.

## Dependencies

Key external dependencies:
- `github.com/zishang520/socket.io/v2`: Real-time communication (Socket.io)
- `github.com/fsnotify/fsnotify`: File system watching
- `github.com/dhowden/tag`: Audio file metadata parsing
- `github.com/golang-jwt/jwt/v5`: JWT token handling
- `github.com/coreos/go-oidc/v3`: OpenID Connect support
- `modernc.org/sqlite`: Pure Go SQLite driver
- `golang.org/x/crypto`: Password hashing (bcrypt)

See `go.mod` for the full dependency list.

## Configuration & Deployment

- Config files: Passed via `--config` flag (default location: `./config`)
- Metadata storage: Passed via `--metadata` flag (default: `./metadata`)
- Important: Reverse proxies must support WebSocket upgrades (Socket.io). Examples in `readme.md` for NGINX, Apache, and Caddy.

## API Documentation

OpenAPI spec is maintained in the `docs/` directory. To regenerate documentation:
```bash
cd docs
redocly bundle root.yaml > bundled.yaml
yq -p yaml -o json bundled.yaml > openapi.json
redocly build-docs openapi.json
cd ..
```

## Common Issues & Patterns

1. **Frontend not updating**: Run `npm run generate` in `frontend/` after changes and rebuild the Go binary.
2. **Socket.io connection issues**: Ensure reverse proxy supports WebSocket upgrades.
3. **Library scanning hangs**: Check file permissions on library folders and FFmpeg/ffprobe availability.
4. **Metadata not fetching**: Verify metadata provider configurations in settings.
