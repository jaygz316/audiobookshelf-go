# Implementation Plan: Live Listening History Trackers

We will implement item-level historical timelines for each user session and map daily listening time stats in the Go backend. We will also expose listening statistics and historical sessions via REST API endpoints and integrate them into the frontend Web UI.

## Proposed Changes

1. **Playback Session Update Logic (Backend)**
   - In `internal/handlers/me.go`, when `PATCH /api/me/progress/{id}` is processed:
     - Find the most recent active playback session for the user and the media item (`userId = ? AND mediaItemId = ?`).
     - Calculate the listening time delta: `currentTime - lastTime` (from the session's `extraData`).
     - If the delta is positive and reasonable (e.g., `< 15` seconds, given progress updates occur every 10 seconds), add it to `timeListened`.
     - Update the session's `extraData` with the new `lastTime`, updated `timeListened`, and fallback `playMethod`/`deviceInfo`.
     - Update the `updatedAt` timestamp of the session to the current time.

2. **New REST API Endpoints (Backend)**
   - **User Listening Stats (`GET /api/users/{id}/listening-stats`)**:
     - Calculates:
       - `totalTime`: Total listening duration in seconds across all sessions for the user.
       - `today`: Total listening duration in seconds today (local or UTC day).
       - `days`: Object mapping date strings (`YYYY-MM-DD`) to total listening time (seconds) on that day.
       - `dayOfWeek`: Object mapping days of the week (0 = Sunday, ..., 6 = Saturday) to total listening time.
       - `items`: Map of library item ID to `{ timeListened: float64, title: string, author: string }`.
       - `recentSessions`: List of the 10 most recent playback sessions with title, author, and time listened.
   - **User Listening Sessions (`GET /api/users/{id}/listening-sessions`)**:
     - Supports pagination via `page` (default 0) and `itemsPerPage` (default 10).
     - Returns a paginated list of sessions for the specified user, sorted by `updatedAt` / `createdAt` DESC.
   - **Me Listening Sessions (`GET /api/me/listening-sessions`)**:
     - Retreives the logged-in user's listening sessions.

3. **Frontend Web UI Integration (Frontend)**
   - In `frontend/index.html` or settings panel, add a dedicated "Stats" tab or section inside Settings or User Profile.
   - Design a premium, high-fidelity statistics dashboard with:
     - Total hours listened.
     - Today's listening time.
     - Visual breakdown of listening time per day of week.
     - A clean list of recent playback sessions (timeline).
   - Fetch stats from `/api/users/{id}/listening-stats` and `/api/users/{id}/listening-sessions`.

4. **Verification & Testing**
   - Write comprehensive unit tests in `internal/handlers/playback_history_test.go`.
   - Write E2E integration test in `e2e/f12_listening_history_test.go` verifying the stats aggregation, session updates, and REST endpoint correctness.
