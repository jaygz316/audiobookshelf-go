# Implementation Plan - WebSocket API Backplane

We will complete the implementation of the WebSocket API Backplane feature to support real-time state synchronization of active playback sessions, including a live Listening Sessions view with real-time additions, updates, deletions, and administrative session termination.

## Proposed Changes

1. **Backend REST API and Socket Broadcasters** (Already partially implemented, verified, and extended):
   - Added REST endpoint `DELETE /api/playback-sessions/:id` for terminating specific playback sessions.
   - Enforced permissions (only the owner or an admin/root can close the session).
   - Hooked up socket emitters:
     - `playback_session_added`
     - `playback_session_updated`
     - `playback_session_removed`
   - Real-time broadcasts configured in:
     - `internal/handlers/me.go` (progress updates)
     - `internal/handlers/misc_handlers.go` (explicit session close)
     - `internal/hls/hls.go` (play session initialization)
   - Covered with robust unit tests (`internal/socket/socket_test.go`, `internal/handlers/playback_sessions_test.go`) and E2E tests (`e2e/f5_playback_test.go`).

2. **Frontend Real-time Synchronization**:
   - Exposed `window.currentUser` on bootstrap to support permission checking on the client side.
   - Subscribed to `playback_session_added`, `playback_session_updated`, and `playback_session_removed` events at the module level in `frontend/js/settings.js`.
   - Enabled client-side in-memory filtering by user so socket updates seamlessly reflect filtered or unfiltered active sessions.
   - Added an **Actions** column in the Listening Sessions tab displaying a "Close Session" button.
   - Secured the "Close Session" action so it is only visible and executable if the current user has appropriate credentials (owner or root/admin).
   - Wired the button to dispatch a `DELETE /api/playback-sessions/:id` HTTP request, triggering immediate local and socket-broadcast state propagation.

## Verification
- Run backend unit tests (`go test ./...`) to verify socket authorization and session deletion.
- Run E2E tests (`go test ./e2e/...`) to verify restricted-access enforcement and session life-cycle endpoints.
