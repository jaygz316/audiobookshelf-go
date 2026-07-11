# Plan: Implement Device & Session Management

## Objective
Implement Device & Session Management to allow administrators and users to view, manage, and revoke active login sessions and authorized device tokens.

## Tasks

### 1. Database Helpers (Go)
- Update `internal/db/users.go` to add:
  - `GetUserSessions(db *sql.DB, userID string) ([]UserSessionDB, error)`: Query active sessions for a user from the `sessions` table.
  - `DeleteSessionByID(db *sql.DB, sessionID string) error`: Remove a session from the `sessions` table.

### 2. Backend API Handlers (Go)
- Update `internal/handlers/users.go` to implement:
  - `GET /api/users/{id}/sessions` (or `GET /api/me/sessions` if ID is "me"): Returns a list of active login sessions for the specified user. Includes IP address, User-Agent, Created time, and whether it represents the current active connection.
  - `DELETE /api/users/{id}/sessions/{sessionId}`: Revokes the specified session. If the revoked session is the current caller's session, they are logged out.
- Update `internal/handlers/routes.go` to register the new endpoints under `registerAuthAndUserRoutes`.

### 3. Frontend UI (HTML/JS)
- Update `frontend/index.html` to add a new tab button/pane for **Login Sessions**.
- Update `frontend/js/settings.js` to:
  - Add navigation support for the `login-sessions` tab.
  - Implement `renderLoginSessionsTab()` to fetch and display the user's active sessions in a clean tabular view.
  - Support administrative filtering (viewing other users' sessions if the logged-in user is root/admin).
  - Add a **Revoke** button next to each session with confirmation. If they revoke their own session, trigger a logout.

### 4. Testing & Verification
- Create `internal/handlers/users_sessions_test.go` with unit tests for the GET and DELETE sessions endpoints.
- Create `e2e/f21_login_sessions_test.go` to verify the E2E lifecycle:
  - Log in to create a session.
  - Fetch active sessions and check current session indicator.
  - Revoke a session and verify it is removed.
  - Try accessing protected endpoints with a revoked session and verify access is denied.
- Run `go test ./...` and `go test ./e2e/...`.
