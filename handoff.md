# Handoff: Audiobookshelf Go Port

## Targeted Task & Accomplishments
- **Target Task**: Path Traversal Audit & JWT / Metrics Security Hardening
- **Accomplishments**:
  - Remediated the remaining 6 critical path traversal gaps identified in the security audit:
    1. **Cover Handler (Cache Serve)**: Verified `cachePath` using `utils.IsSafeFilePath` before serving via `http.ServeFile` in `internal/handlers/library_handlers.go`.
    2. **Cover Handler (URL Download)**: Added `utils.IsSafeFilePath` verification for target destination paths in `downloadCoverFromURL` inside `internal/handlers/library_handlers.go`.
    3. **Public Share Cover Handler**: Sanitized share IDs and restricted served cache paths in `handleGetPublicShareCover` inside `internal/handlers/public_share_handlers.go`.
    4. **SMTP Email E-book Attachments**: Guarded against path traversal in ebook files sent as attachments in `handleSendBookEmail` inside `internal/handlers/email_handlers.go`.
    5. **Podcast Episode Deletion**: Validated path safety with `utils.IsSafeFilePath` before deleting podcast episode files on hard delete in `internal/handlers/podcast_handlers.go`.
    6. **Audio Track Merger**: Validated all source file paths, target output path, and deleted paths using `utils.IsSafeFilePath` inside `internal/handlers/merge_handler.go`.
    7. **HLS Transcoding Stream Manager**: Checked all individual audio track file paths in `LoadOrCreateStream` inside `internal/hls/hls.go`.
  - Hardened authentication boundaries:
    1. **JWT Refresh Token Isolation**: Prevented privilege escalation by rejecting JWT claims of type `refresh` in `AuthMiddleware` and `validateToken` (for Socket.io), ensuring refresh tokens cannot be used as standard API credentials.
    2. **Prometheus Metrics Access Control**: Wrapped the `/metrics` endpoint with authentication middleware and restricted metrics retrieval to `admin` and `root` users to prevent leakage of server and application internals.
    3. **Password Change Session Invalidation**: Modified password update flow in `handleUpdateMePassword` to regenerate the user's permanent token and delete all active sessions, forcing re-authentication across all devices.
    4. **Adversarial Security Test Suite**: Implemented robust unit and integration tests in `internal/handlers/security_adversarial_test.go` verifying the token type restrictions, metrics protection, and session revocation.

## Outstanding Work / Next Gaps
- Perform a review of other privilege checks or admin-only routes to ensure no other unauthorized endpoints are exposed.

## Next Steps
- Verify if any other files or folder APIs (e.g. library creation, backups, etc.) need explicit path validation checks.
