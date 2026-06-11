# Project: Audiobookshelf-go Frontend Rewrite

## Architecture
- **Backend**: Go HTTP server (`main.go`) serving API endpoints, WebSockets (`/socket.io/`), and HLS streams.
- **Frontend**: Vanilla HTML/CSS/JS frontend embedded in the Go binary using `go:embed`.
- **Data Flow**:
  - Direct HTTP calls (using browser `fetch`) for REST APIs.
  - Custom WebSocket wrapper (Socket.io client-compatible protocol over standard WebSocket connection) for real-time progress syncing.
  - Native HTML5 Audio element for book and podcast playback (supports HLS/direct audio streams).
- **Security**: Cookie or Header-based JWT auth tokens synced between HTTP request headers and WebSocket handshakes. OIDC flow delegates authentication redirects.

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| 1 | E2E Testing Suite | Design and build the opaque-box test suite for features, boundaries, pairwise, and application workloads. Publishes `TEST_READY.md`. | None | DONE (Conv: 82cd8ed4-4508-45e7-8c8e-e571df70c5e4) |
| 2 | Exploration & Layout Mockup | Audit Nuxt frontend assets, copy icon/texture files to target asset folder, construct vanilla HTML/CSS framework mimicking original wood bookshelf aesthetics. | None | DONE (Conv: 212786ae-8d9f-4e59-8366-aa26a23f573d) |
| 3 | Auth & Home Module | Implement login/logout forms (including OIDC auth triggers), session persistence, library retrieval, and personalized home view. | M2 | DONE (Conv: 9997294d-99c2-4948-92f0-67c66fd875c0) |
| 4 | Playback & Sockets | Implement the audio player UI and playback engine, sync audio progress, and establish WebSockets to handle playback states. | M3 | DONE (Conv: 098c4469-253f-46b4-a7dd-85bb549d6a65) |
| 5 | Settings & Management | Build forms for library scanning, server settings, playlist creation/deletion, and collection organization. | M3 | DONE (Conv: c34c1b88-3350-4c19-bab4-2267c0cd821b) |
| 6 | Go Integration & Embedding | Embed frontend using Go `embed`, rewrite backend routing to serve embedded assets, support custom subpath routing. | M2, M3, M4, M5 | IN_PROGRESS |
| 7 | Verification & Hardening | Phase 1: Verify passing 100% of the E2E test suite. Phase 2: Run white-box adversarial coverage hardening. | M1, M6 | PLANNED |

## Interface Contracts
### Client ↔ Server Auth
- Login API: `POST /api/login` returning JSON containing `user` object and a token.
- Authentication Token: Passed via `Authorization: Bearer <token>` header on REST requests.
- OIDC redirect: Redirects to `/auth/openid` to trigger OIDC flow.
- OIDC callback: Native handling via `/auth/openid/callback` resulting in redirection back to app root with auth cookie/token.

### Client ↔ Server WebSockets
- WebSocket URL: Standard Socket.io-compliant handshake on `/socket.io/` or base path.
- Initial handshake: Connects, auth token passed inside custom query params or authentication headers.
- Key events:
  - Client emits `play`, `pause`, `progress` with session info.
  - Server emits updates on scanning progress, backup tasks, etc.

## Code Layout
- Existing backend code files reside at project root and under `/internal`.
- Vanilla frontend files will be placed under `/frontend` (e.g. `index.html`, `app.js`, `styles.css`, `assets/`).
- The Go backend will embed files in `frontend/` using `//go:embed frontend/*`.
