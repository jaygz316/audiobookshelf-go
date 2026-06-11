# E2E Test Infrastructure & Catalog Design (TEST_INFRA.md)

This document provides the blueprint for the Audiobookshelf Go port end-to-end (E2E) testing suite, detailing test strategies, environments, and a complete catalog of test cases organized into four tiers.

---

## 1. Introduction & Strategy

The rewrite of the Audiobookshelf frontend into a Go-embedded, dependency-free client requires robust E2E testing to ensure:
1. **Behavioral Parity:** The Go-embedded application behaves identically to the original Nuxt client.
2. **Opaque-Box Verification:** The frontend integration, REST APIs, WebSockets, and media streaming work seamlessly together under realistic user conditions.
3. **Security and Permissions Enforcement:** User permission scopes (downloads, explicit content, libraries, tags) are strictly enforced at the endpoint and WebSocket layer.

### Testing Tools & Architecture
- **Test Runner:** Playwright (Node/TypeScript) or Go integration tests with standard HTTP clients and WebSocket libraries (e.g., standard `gorilla/websocket` or socket.io clients).
- **Environment:** Containerized environment (Docker Compose) containing the compiled Go binary, a mock OIDC Identity Provider (e.g., Keycloak or a custom lightweight OAuth provider mock), and pre-populated media files.
- **SQLite Database Mocking:** Pre-configured database states corresponding to fresh installation, initialized admin, and populated libraries to allow tests to run against deterministic datasets.

---

## 2. Feature Inventory (N = 8)

The following core modules are targeted for E2E testing, mapping backend routes and socket events to frontend user interfaces.

| Ref | Core Feature | Affected Backend Files | Key Endpoints / Events |
| :--- | :--- | :--- | :--- |
| **F1** | Local Auth & Session Management | `main.go`, `auth.go`, `users.go` | `POST /login`, `POST /logout`, `POST /init`, `POST /auth/refresh`, `POST /api/authorize`, cookie `refresh_token` |
| **F2** | OIDC Federated Auth | `internal/auth/auth.go`, `auth.go` | `GET /auth/openid`, `/auth/openid/callback`, `/auth/openid/mobile-redirect` |
| **F3** | Library & Folder Administration | `main.go`, `db.go` | `POST /api/libraries`, `GET /api/libraries`, `PATCH /api/libraries/:id`, `DELETE /api/libraries/:id`, `/api/filesystem/pathexists` |
| **F4** | Library Scanning & Watching Tasks | `scanner.go`, `watcher.go`, `main.go` | `POST /api/libraries/:id/scan`, `GET /api/tasks`, socket event `cancel_scan`, task socket broadcasts |
| **F5** | Catalog Retrieval, Searching & Filtering | `main.go`, `db.go`, `authors_series.go` | `GET /api/libraries/:id/items`, `/api/libraries/:id/personalized`, `/api/search/books`, `/api/tags/rename`, `/api/genres/rename` |
| **F6** | Content Access & Downloading | `main.go`, `db.go` | `GET /api/items/:id/cover`, `/api/items/:id/download`, `/api/items/:id/ebook`, cover image formats and caching |
| **F7** | Audio Playback & HLS Transcoding | `hls.go` | `GET /hls/:streamId/output.m3u8`, `/hls/:streamId/output-:num.ts`, dynamic seek resets, copy codec fallbacks |
| **F8** | WebSocket Progress Sync & Bookmarks | `socket.go` | WS `/socket.io/`, `auth` event, `user_item_progress_updated`, bookmarks REST `/api/me/item/:id/bookmark` |

---

## 3. E2E Test Catalog

### Tier 1: Feature Coverage (Happy Path & Core Flow)

#### F1: Local Auth & Session Management
- **Test Case T1.1.1 (Fresh Install Setup):**
  - *Context:* Fresh server database without any users.
  - *Action:* Send `POST /init` with username `admin` and password `Password123`.
  - *Verification:* Expect HTTP 200 OK. Verify database now contains a user with `type = 'root'` and the UI redirects to the login screen.
- **Test Case T1.1.2 (Standard User Login):**
  - *Context:* User `testuser` is configured in SQLite.
  - *Action:* Send `POST /login` with credentials.
  - *Verification:* Expect HTTP 200 containing JSON with a `user` session object, a `token` (JWT access token), and a `refresh_token` set in a Secure, HttpOnly cookie.
- **Test Case T1.1.3 (Token Refresh Rotation):**
  - *Context:* Logged in user with valid `refresh_token` cookie.
  - *Action:* Send `POST /auth/refresh` without access token in headers.
  - *Verification:* Expect HTTP 200 containing a new JWT access token, and a newly rotated `refresh_token` set in cookies.
- **Test Case T1.1.4 (User Logout Flow):**
  - *Context:* Logged in user session exists in SQLite.
  - *Action:* Send `POST /logout` with active refresh token.
  - *Verification:* Expect HTTP 200. Verify the refresh token cookie is expired/cleared, and the session row is deleted from the `sessions` table in the database.
- **Test Case T1.1.5 (Session Authorization Validation):**
  - *Context:* Logged in user with active JWT.
  - *Action:* Send `POST /api/authorize` with `Authorization: Bearer <token>`.
  - *Verification:* Expect HTTP 200 returning the user metadata dictionary.

#### F2: OIDC Federated Auth
- **Test Case T2.1.1 (OIDC Login Redirect):**
  - *Context:* OIDC configuration is active in settings.
  - *Action:* Visit `/auth/openid` in the browser.
  - *Verification:* Verify redirection to the configured external OIDC issuer URL with correct `client_id`, `response_type=code`, and `scope=openid profile email`.
- **Test Case T2.1.2 (OIDC Callback Flow):**
  - *Context:* OIDC login completed at provider.
  - *Action:* Receive redirect at `/auth/openid/callback` with `code` and matching `state`.
  - *Verification:* Expect redirect back to app root `/` with set JWT and refresh cookies. Verify user session is initialized in SQLite.
- **Test Case T2.1.3 (OIDC Mobile Redirect):**
  - *Context:* Mobile client initiates OIDC.
  - *Action:* Receive redirect at `/auth/openid/mobile-redirect` with authorization code.
  - *Verification:* Expect a redirect to `audiobookshelf://oauth?token=...` or custom mobile schema.
- **Test Case T2.1.4 (OIDC Auto-Register User):**
  - *Context:* `authOpenIDAutoRegister` is enabled. OIDC callback provides a new user profile.
  - *Action:* Complete login callback.
  - *Verification:* Verify a new user is created in the `users` table with default standard permissions.
- **Test Case T2.1.5 (OIDC Match Existing Account):**
  - *Context:* User `alice` exists locally. OIDC claims email `alice@example.com` matching Alice.
  - *Action:* Run OIDC login matching by email.
  - *Verification:* Verify OIDC login binds to Alice's existing local profile without duplicating accounts.

#### F3: Library & Folder Administration
- **Test Case T3.1.1 (Library Creation):**
  - *Context:* Authenticated Admin.
  - *Action:* Send `POST /api/libraries` with mediaType `book`, folders `["/data/audiobooks"]`, and name `My Books`.
  - *Verification:* Expect HTTP 200. Verify row in `libraries` and `libraryFolders` table.
- **Test Case T3.1.2 (Library Retrieval):**
  - *Context:* Authenticated User.
  - *Action:* Send `GET /api/libraries`.
  - *Verification:* Expect list of libraries including mapped folders and display configuration.
- **Test Case T3.1.3 (Library Folder Update):**
  - *Context:* Authenticated Admin.
  - *Action:* Send `PATCH /api/libraries/:id` to add folder `["/data/archive"]`.
  - *Verification:* Expect HTTP 200. Verify the database matches updated mappings.
- **Test Case T3.1.4 (Library Deletion):**
  - *Context:* Authenticated Admin.
  - *Action:* Send `DELETE /api/libraries/:id`.
  - *Verification:* Expect HTTP 200. Verify the library folder mappings and library items in the SQLite DB are removed.
- **Test Case T3.1.5 (Library Stats Aggregation):**
  - *Context:* Populated library.
  - *Action:* Send `GET /api/libraries?include=stats`.
  - *Verification:* Verify returned payload contains calculated total size, duration, item count, and track counts.

#### F4: Library Scanning & Watching Tasks
- **Test Case T4.1.1 (Manual Scan Trigger):**
  - *Context:* Mapped directory contains new audiobook files.
  - *Action:* Send `POST /api/libraries/:id/scan`.
  - *Verification:* Expect HTTP 200. Verify SQLite logs scan task started.
- **Test Case T4.1.2 (Live Socket Scan Progress):**
  - *Context:* Active scan task running.
  - *Action:* Connect WebSocket and listen for events.
  - *Verification:* Verify receipt of `task_started`, `task_progress`, and `task_finished` events.
- **Test Case T4.1.3 (Active Task Listing):**
  - *Context:* Active library scan.
  - *Action:* Send `GET /api/tasks`.
  - *Verification:* Expect a list containing the active scan task, progress percentage, and description.
- **Test Case T4.1.4 (Filesystem Watcher Ingestion):**
  - *Context:* Directory watcher is running on library folders.
  - *Action:* Write a new directory and audio file to the folder.
  - *Verification:* Verify the server automatically initiates a scan and imports the new audiobook.
- **Test Case T4.1.5 (Scan Cancellation):**
  - *Context:* Running library scan.
  - *Action:* Emit `cancel_scan` event with library ID via WebSocket.
  - *Verification:* Verify scan stops immediately and task finishes with canceled status.

#### F5: Catalog Retrieval, Searching & Filtering
- **Test Case T5.1.1 (Filtered Library Items):**
  - *Context:* Library with multiple books.
  - *Action:* Send `GET /api/libraries/:id/items?sort=addedAt&desc=1&limit=10`.
  - *Verification:* Verify JSON results are sorted by date and paginate correctly.
- **Test Case T5.1.2 (Personalized Home Shelves):**
  - *Context:* User has books in-progress.
  - *Action:* Send `GET /api/libraries/:id/personalized`.
  - *Verification:* Expect shelves like "Continue Listening" and "Continue Reading" listing correct books.
- **Test Case T5.1.3 (Tag Renaming Sync):**
  - *Context:* Multiple books contain tag `Sci-Fi`.
  - *Action:* Send `POST /api/tags/rename` with old `Sci-Fi` and new `Science Fiction`.
  - *Verification:* Expect HTTP 200. Verify the database updates associations across all library items.
- **Test Case T5.1.4 (Custom Metadata Provider Fetch):**
  - *Context:* Mapped book lacks metadata.
  - *Action:* Send search query via Audnexus custom provider.
  - *Verification:* Verify returned structured metadata containing authors, title, description, and cover image options.
- **Test Case T5.1.5 (Library Authors and Series Index):**
  - *Context:* Populated database.
  - *Action:* Send `GET /api/libraries/:id/authors` and `GET /api/libraries/:id/series`.
  - *Verification:* Verify alphabetized indexes return with correct entity counts.

#### F6: Content Access & Downloading
- **Test Case T6.1.1 (Raw Cover Delivery):**
  - *Context:* Library item with cover.
  - *Action:* Send `GET /api/items/:id/cover?raw=1`.
  - *Verification:* Verify HTTP 200 with appropriate Image Content-Type matching the asset file on disk.
- **Test Case T6.1.2 (WebP Image Resizing Caching):**
  - *Context:* Mapped cover asset.
  - *Action:* Send `GET /api/items/:id/cover?width=400&format=webp`.
  - *Verification:* Expect resized WebP image. Verify cached resized file exists under `metadata/cache/covers`.
- **Test Case T6.1.3 (Single File Download):**
  - *Context:* User with download permissions.
  - *Action:* Send `GET /api/items/:id/download`.
  - *Verification:* Verify HTTP header `Content-Disposition: attachment; filename="[name]"` and full audio file download.
- **Test Case T6.1.4 (Directory On-The-Fly Zip Download):**
  - *Context:* Audiobook represented by a multi-file directory.
  - *Action:* Send `GET /api/items/:id/download`.
  - *Verification:* Verify returned content has mime-type `application/zip` containing all nested audio tracks.
- **Test Case T6.1.5 (Ebook Resource Serving):**
  - *Context:* Library item is an EPUB book.
  - *Action:* Send `GET /api/items/:id/ebook`.
  - *Verification:* Expect successful file download or parsed e-book resource structure.

#### F7: Audio Playback & HLS Transcoding
- **Test Case T7.1.1 (HLS Session Initialization):**
  - *Context:* Playback session requested.
  - *Action:* Send `GET /hls/:streamId/output.m3u8`.
  - *Verification:* Expect VOD playlist format (`#EXTM3U`, `#EXT-X-TARGETDURATION`, segments lists output-0.ts).
- **Test Case T7.1.2 (Sequential Segment Downloading):**
  - *Context:* Active transcoding session.
  - *Action:* Download `/hls/:streamId/output-0.ts`.
  - *Verification:* Expect HTTP 200 with MPEG-TS audio file stream.
- **Test Case T7.1.3 (Transcode Fallback to AAC):**
  - *Context:* Audiobook file uses format unsupported by direct copying (e.g., FLAC, Opus).
  - *Action:* Initialize stream playback.
  - *Verification:* Verify FFmpeg falls back to forced AAC encoding (`-c:a aac`) and plays cleanly.
- **Test Case T7.1.4 (Streaming Socket Events):**
  - *Context:* Client connects to socket.io.
  - *Action:* Start stream.
  - *Verification:* Verify client receives `stream_open`, `stream_progress`, and `stream_ready`.
- **Test Case T7.1.5 (Clean Session Shutdown):**
  - *Context:* Stream is closed by client.
  - *Action:* Terminate session.
  - *Verification:* Verify the corresponding FFmpeg process group is killed, stream directory under `streams/` is purged, and socket emits `stream_closed`.

#### F8: WebSocket Progress Sync & Bookmarks
- **Test Case T8.1.1 (Socket Authentication Handshake):**
  - *Context:* Client establishes WebSocket connection.
  - *Action:* Emit `auth` event with valid JWT token.
  - *Verification:* Receive `init` event detailing userId and user profile.
- **Test Case T8.1.2 (Online User Administration):**
  - *Context:* Admin socket active.
  - *Action:* A user logs in and establishes a socket connection.
  - *Verification:* Expect admin socket to receive `user_online` event.
- **Test Case T8.1.3 (Playback Progress Emission):**
  - *Context:* Active socket connection.
  - *Action:* Emit `progress` event with audio position.
  - *Verification:* Expect progress table updates in SQLite.
- **Test Case T8.1.4 (Multi-Client Progress Synchronization):**
  - *Context:* User has two active connections (mobile and browser).
  - *Action:* Emit progress update from mobile.
  - *Verification:* Expect browser socket to receive `user_item_progress_updated` event syncing the playback slider.
- **Test Case T8.1.5 (Bookmark Synchronization):**
  - *Context:* Logged in user.
  - *Action:* Send `POST /api/me/item/:id/bookmark` with time.
  - *Verification:* Verify bookmark row added in DB and emitted to all active user sockets.

---

## Tip 2: Boundary & Corner Cases (Error Handling & Edge Behaviors)

#### F1: Local Auth & Session Management
- **Test Case T1.2.1 (Duplicate Setup Block):**
  - *Context:* Root user already initialized in DB.
  - *Action:* Send `POST /init` with setup credentials.
  - *Verification:* Expect HTTP 500 error payload `{"error": "Root user already exists"}` or HTTP 403 Forbidden.
- **Test Case T1.2.2 (Incorrect Credentials Login):**
  - *Context:* User exist in DB.
  - *Action:* Send login request with incorrect password.
  - *Verification:* Expect HTTP 401 with `{"error": "Invalid username or password"}`.
- **Test Case T1.2.3 (JWT Signature Violation):**
  - *Context:* Endpoint requires authentication.
  - *Action:* Call `/api/me` using a JWT signed with an incorrect key.
  - *Verification:* Expect HTTP 401 Unauthorized.
- **Test Case T1.2.4 (Refresh Token Reuse Violation):**
  - *Context:* Token was rotated previously, and grace period (60s) has expired.
  - *Action:* Trigger a second refresh request using the old rotated refresh token.
  - *Verification:* Expect HTTP 400 Bad Request or HTTP 401 Unauthorized.
- **Test Case T1.2.5 (Deactivated Account Login):**
  - *Context:* User has `isActive = 0` in SQLite database.
  - *Action:* Send valid username/password credentials login request.
  - *Verification:* Expect HTTP 401 Unauthorized indicating user is inactive.

#### F2: OIDC Federated Auth
- **Test Case T2.2.1 (OIDC State Parameter Forgery):**
  - *Context:* OIDC login sequence active.
  - *Action:* Callback triggers with a state parameter not matching the stored session state.
  - *Verification:* Expect HTTP 400 Bad Request or HTTP 401 Unauthorized.
- **Test Case T2.2.2 (Unconfigured OIDC Request):**
  - *Context:* Settings table has OIDC values empty.
  - *Action:* Trigger OIDC auth route `/auth/openid`.
  - *Verification:* Expect HTTP 400 with a config error message.
- **Test Case T2.2.3 (OIDC Provider Unreachable):**
  - *Context:* External provider is offline or returns server error (502).
  - *Action:* Trigger callback.
  - *Verification:* Verify server falls back gracefully to a descriptive error page or JSON error (500).
- **Test Case T2.2.4 (Registration Validation Failures):**
  - *Context:* Auto-register on OIDC with malformed username claim.
  - *Action:* Callback parses token.
  - *Verification:* Graceful validation rejection without corrupting database or causing server crashes.
- **Test Case T2.2.5 (OIDC Mobile Redirect Spoofing):**
  - *Context:* Callback processed for mobile redirect.
  - *Action:* Pass redirect URL using a non-whitelisted protocol or host.
  - *Verification:* Expect validation reject with HTTP 400 Bad Request.

#### F3: Library & Folder Administration
- **Test Case T3.2.1 (Invalid Folder Ingestion):**
  - *Context:* Admin creating library.
  - *Action:* Mapped directory does not exist or has no read permissions.
  - *Verification:* Verify system rejects folder or throws validation warning, preventing folder mapping.
- **Test Case T3.2.2 (Missing Library Updates):**
  - *Context:* Update library endpoint.
  - *Action:* Send PATCH update command targeting a non-existent UUID.
  - *Verification:* Expect HTTP 404 Not Found.
- **Test Case T3.2.3 (Non-Admin Library Mutation):**
  - *Context:* Non-admin authenticated user.
  - *Action:* Send `DELETE /api/libraries/:id`.
  - *Verification:* Expect HTTP 403 Forbidden.
- **Test Case T3.2.4 (Path Traversal in Path Check):**
  - *Context:* Endpoint `/api/filesystem/pathexists`.
  - *Action:* Pass path values containing `../../etc/passwd` or outside target scopes.
  - *Verification:* Verify path validation blocks traversal and returns false/error.
- **Test Case T3.2.5 (Access Restricted Libraries):**
  - *Context:* User has permission restrict to library A.
  - *Action:* Query items from library B.
  - *Verification:* Expect HTTP 403 Forbidden.

#### F4: Library Scanning & Watching Tasks
- **Test Case T4.2.1 (Scan Missing Library):**
  - *Context:* Trigger library scan.
  - *Action:* Call scan endpoint with invalid library ID.
  - *Verification:* Expect HTTP 404 Not Found.
- **Test Case T4.2.2 (Unauthorized Scan Triggers):**
  - *Context:* Standard user lacking library scanning permissions.
  - *Action:* Send scan post request.
  - *Verification:* Expect HTTP 403 Forbidden.
- **Test Case T4.2.3 (Watcher Permission Failures):**
  - *Context:* File watcher encounters directories with zero permissions.
  - *Action:* Simulate file event on restricted files.
  - *Verification:* Ensure watcher catches read permissions gracefully and logs warning without stopping task runner.
- **Test Case T4.2.4 (Standard User Cancel Scan):**
  - *Context:* Active library scan.
  - *Action:* Standard user socket emits `cancel_scan`.
  - *Verification:* Ensure event is ignored. Socket client receives unauthorized event rejection.
- **Test Case T4.2.5 (Scan Queue Race):**
  - *Context:* Scan is actively executing.
  - *Action:* Trigger a second scan request.
  - *Verification:* Verify request is deduped or queues gracefully instead of spawning duplicate scanner processes.

#### F5: Catalog Retrieval, Searching & Filtering
- **Test Case T5.2.1 (Personalized Empty Catalog):**
  - *Context:* Fresh library, zero content.
  - *Action:* Query `/api/libraries/:id/personalized`.
  - *Verification:* Expect HTTP 200 with empty arrays inside shelves. No server crashes.
- **Test Case T5.2.2 (Invalid Sorting Columns):**
  - *Context:* Catalog query.
  - *Action:* Request sorting by column name that does not exist in SQLite tables.
  - *Verification:* Fallback safely to sort by alphabetical title.
- **Test Case T5.2.3 (Tag Rename Cascading Race):**
  - *Context:* Massive database (10k items) containing target tags.
  - *Action:* Rename tag while items are actively queried.
  - *Verification:* SQL transaction handles isolation cleanly without locking up API routing.
- **Test Case T5.2.4 (Metadata Provider Failure / Timeout):**
  - *Context:* Provider lookup.
  - *Action:* Provider API endpoint is blocked or times out.
  - *Verification:* Return empty metadata search matches cleanly after timeout.
- **Test Case T5.2.5 (Search Mismatched Provider Types):**
  - *Context:* Mapped provider is podcast-only.
  - *Action:* Call provider search API targeting books.
  - *Verification:* Graceful validation response.

#### F6: Content Access & Downloading
- **Test Case T6.2.1 (Missing Item Cover):**
  - *Context:* Book has no cover image mapped.
  - *Action:* Send `GET /api/items/:id/cover`.
  - *Verification:* Expect HTTP 404 or default placeholder image.
- **Test Case T6.2.2 (Invalid Resize Query Parameters):**
  - *Context:* Resize query.
  - *Action:* Pass `width=-200` or `format=invalid_mime`.
  - *Verification:* Server handles parameter validation, falls back to original format and size cleanly.
- **Test Case T6.2.3 (Forbidden File Downloads):**
  - *Context:* User permissions configuration has `download = false`.
  - *Action:* Send `GET /api/items/:id/download`.
  - *Verification:* Expect HTTP 403 Forbidden.
- **Test Case T6.2.4 (Empty Directory Zip Generation):**
  - *Context:* Mapped directory has no files.
  - *Action:* Request zip download.
  - *Verification:* Handle empty zip gracefully, returning structured error or valid empty archive.
- **Test Case T6.2.5 (Path Traversal in Ebook Access):**
  - *Context:* Ebook endpoint.
  - *Action:* Pass path containing `../../../etc/shadow` in fileId parameter.
  - *Verification:* Verify server sanitizes paths, returning HTTP 404 or 400.

#### F7: Audio Playback & HLS Transcoding
- **Test Case T7.2.1 (Segment Request for Dead Stream):**
  - *Context:* Accessing transcoder outputs.
  - *Action:* Send request for `/hls/nonexistent_id/output-0.ts`.
  - *Verification:* Expect HTTP 404 Not Found.
- **Test Case T7.2.2 (Seek Miss Reset):**
  - *Context:* Active playback, transcode progress is at segment 10.
  - *Action:* Client seeks forward to segment 50 (well outside transcode buffer).
  - *Verification:* Expect server to detect seek-miss, kill current FFmpeg process, start a new transcode process at segment 45 (seek-back buffer of 5 segments), and broadcast `stream_reset` to socket.
- **Test Case T7.2.3 (Seek Backward Reset):**
  - *Context:* Active playback, transcode is at segment 30.
  - *Action:* Client seeks back to segment 5.
  - *Verification:* Expect server to restart FFmpeg from segment 0 (or adjusted start time) and broadcast reset event.
- **Test Case T7.2.4 (Simultaneous Segment Requests):**
  - *Context:* Startup buffering.
  - *Action:* Request segments 0, 1, and 2 in immediate parallel while transcoding starts.
  - *Verification:* Server returns ready segments immediately, and responds with 404 cleanly for segments not yet transcoded, which client handles by retrying.
- **Test Case T7.2.5 (Orphan Stream Purging):**
  - *Context:* Playback session is abandoned without clean close (e.g. browser crash).
  - *Action:* Background cleanup worker runs.
  - *Verification:* Verify stream session is terminated and directories are deleted if no requests are received for 36 hours.

#### F8: WebSocket Progress Sync & Bookmarks
- **Test Case T8.2.1 (Unauthenticated Socket Actions):**
  - *Context:* Socket connection established but `auth` event not yet sent.
  - *Action:* Emit `progress` or `search_covers` over socket.
  - *Verification:* Expect server to drop payload or close connection.
- **Test Case T8.2.2 (Invalid Token Handshake):**
  - *Context:* Handshake.
  - *Action:* Emit `auth` with malformed string.
  - *Verification:* Expect server to emit `auth_failed` event.
- **Test Case T8.2.3 (Dirty Disconnection Handling):**
  - *Context:* User socket disconnects abruptly (simulate TCP drop).
  - *Action:* Check server online lists.
  - *Verification:* Verify user status transitions to offline, and any active search tasks of this client are cancelled.
- **Test Case T8.2.4 (Permission Sync Update):**
  - *Context:* User has active socket connection.
  - *Action:* Admin restricts user access permissions (e.g., blocks a library).
  - *Verification:* Socket authority detects database change, filters output lists, and restricts item emissions.
- **Test Case T8.2.5 (Administrative Event Spoofing):**
  - *Context:* Standard user socket connection.
  - *Action:* Emit `message_all_users` or `set_log_listener`.
  - *Verification:* Verify Socket Authority drops the event and returns warning logs.

---

## Tier 3: Cross-Feature Combinations (Pairwise Interaction Cases)

Pairwise interaction tests verify the handoffs and state changes across different modules.

#### 1. REST Database Restore x WebSocket Session Invalidation (F1 x F8 x Backup)
- *Scenario:* Admin initiates a system restore via `POST /api/backups/:id/apply`.
- *Verification:* The server closes active database locks, swaps SQLite database files, and apply backup. Verify that all connected user sockets are forcibly closed/invalidated, requiring clients to re-authenticate against the newly restored credentials.

#### 2. Library Creation x Filesystem Watcher Scan Trigger (F3 x F4)
- *Scenario:* Admin adds a new library folder mapping in settings.
- *Verification:* The server initializes a folder listener via the filesystem watcher. Confirm that writing a new media file into this folder triggers an automatic scan task, registering library items in SQLite and notifying the UI via `task_started` / `task_progress` WebSocket events.

#### 3. Scan Ingestion x Content Accessibility Filters (F4 x F5)
- *Scenario:* Library scan completes, ingest files marked as `explicit` or containing specific tags.
- *Verification:* Query catalog using accounts with restricted access scopes (e.g. `CanAccessExplicitContent = false`). Verify that newly-scanned items are hidden in `/api/libraries/:id/items` and `/api/libraries/:id/personalized` queries for restricted users, but visible to admins.

#### 4. Item Catalog Deletion x Real-Time Session Invalidation (F5 x F8)
- *Scenario:* Admin deletes an audiobook that is actively being played by a user on a mobile app.
- *Verification:* The server processes deletion, removes SQLite references, and emits `item_removed` via Socket. The user's client receives this event, stops active HLS audio playback, cleans local states, and returns to the home view.

#### 5. HLS Segment Seek Request x Live Progress Bookmark Synchronization (F7 x F8)
- *Scenario:* User seeks forward in an active audiobook stream (causes HLS seek-miss and FFmpeg reset).
- *Verification:* The server terminates active transcode loops, restarts transcoding, and writes the updated playback position into the SQLite progress tables. Verify that all of the user's other active socket sessions receive `user_item_progress_updated` syncing their playback position.

#### 6. Admin Settings Mutation x User Session Permission Revocation (F1 x F6)
- *Scenario:* Admin updates a user's permissions via `PATCH /api/users/:id`, disabling download rights (`CanDownload = false`).
- *Verification:* The server writes settings to the DB and emits `user_updated` to the user's socket connection. Verify that any subsequent download attempts (e.g. `GET /api/items/:id/download`) by that user return HTTP 403 Forbidden.

#### 7. OIDC Registration x Group Permission Mapping (F2 x F3)
- *Scenario:* A new user logs in via OIDC, and OIDC claims place them in group `Audiobook-Listeners`.
- *Verification:* The server auto-registers the account and maps group permissions to restrict access to only library ID `1`. Verify that requests to `/api/libraries` return only library ID `1`.

#### 8. HLS Playback Segments x Token Expiration Recovery (F7 x F1)
- *Scenario:* Client is playing HLS stream using token parameters in segment URLs, and the token expires.
- *Verification:* Verify that the client refreshes its session via `/auth/refresh` and subsequent segment requests use the updated token, avoiding playback failure or stream restarts.

---

## Tier 4: Real-World Application Scenarios (End-to-End Workloads)

Real-world application tests trace user stories across the system to simulate realistic production usage.

#### 1. Multi-Device Playback Handover & Session Resumption
- *Description:* Simulates a user listening on a mobile client, stopping, and resuming on a desktop web browser.
- *Sequence:*
  1. Login on Mobile Client; establish socket connection and authenticate.
  2. Start playback of Audiobook A. Request HLS segments, and emit progress coordinates.
  3. Pause playback at timestamp `01:15:30` (Mobile emits pause socket event, database writes position).
  4. User opens Desktop Web Browser, logs in, and loads main dashboard.
  5. Dashboard fetches personalized shelves: verify "Continue Listening" lists Audiobook A at `01:15:30`.
  6. Click Play on Desktop. HLS transcode initializes starting at `01:15:30` (with seek-back buffer beginning transcode at `01:15:00`). Playback resumes seamlessly.

#### 2. Bulk Media Ingestion, Cataloging, and Feeds Publication
- *Description:* Ingestion of a massive file load, verification of database persistence, metadata enrichment, and feed generation.
- *Sequence:*
  1. Bulk copy audiobooks to library folders.
  2. Trigger library scan. Watch background scan task execute.
  3. Verify metadata provider queries successfully enrich empty records with details (author, description, cover images).
  4. Query `/api/libraries/:id/items` with search filters to locate specific scanned titles.
  5. Access podcast RSS feeds endpoint `/feed/:slug` and verify valid XML output is served.

#### 3. Offline Mode Playback Sync Reconciliation
- *Description:* Handles network drops and synchronization when connection returns.
- *Sequence:*
  1. Start active playback session.
  2. Disconnect socket connection and block REST API access (simulate network offline).
  3. UI stores progress markers in localStorage during offline playback (moving from `00:10:00` to `00:25:00`).
  4. Network connection restored.
  5. Re-authenticate WebSocket connection. Emit offline-buffered progress payload.
  6. Verify server handles reconciliation, saves progress to SQLite database, and updates bookmarks cleanly.

#### 4. Disconnect/Decommission & Restore Recovery
- *Description:* Validates settings configurations, server backup creation, content loss, and complete restore.
- *Sequence:*
  1. Admin configures custom metadata settings and prefix configurations.
  2. Admin triggers system backup creation via `POST /api/backups`. Verify backup package is saved.
  3. Simulate fatal state by deleting libraries and removing rows from database.
  4. Admin apply restore.
  5. System restarts database connections, re-indexes filesystem paths, and restores all metadata, settings, and users. Verify catalog returns to original configured state.

#### 5. Parental Restrictions and Content Filtering
- *Description:* Validates content restrictions are strictly enforced under various access conditions.
- *Sequence:*
  1. Admin creates library with mixed content (explicit and clean items).
  2. Admin sets up user `child` with permissions `AccessExplicitContent = false` and library mapping restricted to only Library A.
  3. Log in as `child`. Verify home page, search queries, cover requests, and downloads return only clean items in Library A.
  4. Direct request bypass check: try accessing an explicit cover image or file download directly using URL parameters. Verify request returns 403 or 404.

#### 6. Multi-User Concurrent Playback Stress Load
- *Description:* Simulates concurrent transcoding loads to evaluate process management stability.
- *Sequence:*
  1. Spawn parallel HTTP sessions starting playback of different multi-file audiobooks.
  2. Verify distinct FFmpeg transcode processes spawn.
  3. Verify segments are served within timeout requirements without locking SQLite.
  4. Sessions send pause and disconnect. Verify corresponding FFmpeg process groups are killed promptly, freeing system resources.

---

## 4. Verification Methods

This testing strategy can be verified independently using the following testing steps.

### Setup and Compilation
1. Compile the Go backend binary:
   ```bash
   go build -o audiobookshelf-server .
   ```
2. Launch the server in development mode:
   ```bash
   ./audiobookshelf-server -c ./test_config -m ./test_metadata -p 3333
   ```

### Running the Integration Tests
1. Native Go tests cover backend handlers and E2E scenarios. Run:
   ```bash
   go test -v ./e2e/...
   ```
