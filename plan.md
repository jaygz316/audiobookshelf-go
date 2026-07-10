# Implementation Plan: OPDS Feed Server

We will implement a fully compatible OPDS 1.2 catalog feed server in the Go backend, enable HTTP Basic Authentication for OPDS clients in the middleware, write robust unit/integration tests, and expose the OPDS feed link in the Web UI settings panel.

## Proposed Changes

1. **HTTP Basic Authentication Support (Middleware)**
   - In `internal/handlers/middleware.go`, check if `Authorization` header is `Basic <credentials>`.
   - Retrieve the user via `idb.GetUserFullByUsername` and verify the password using `bcrypt.CompareHashAndPassword`.
   - If valid, map the user to `core.UserSession` and inject into context.
   - If credentials are empty or invalid and the path is an OPDS endpoint (`/opds`), return `WWW-Authenticate: Basic realm="Audiobookshelf OPDS"` with a `401 Unauthorized` status to prompt the client for credentials.

2. **OPDS Feed Catalog Endpoints (Backend)**
   - Create `internal/handlers/opds.go` to handle OPDS feed routes.
   - **Root Catalog (`/opds` or `/opds/v1.2/catalog`)**:
     - Lists all libraries the user is authorized to access.
     - For each library, returns a subsection navigation link pointing to `/opds/v1.2/libraries/{libraryID}`.
   - **Library Feed (`/opds/v1.2/libraries/{libraryID}`)**:
     - Returns navigation links:
       - All items: `/opds/v1.2/libraries/{libraryID}/all`
       - Recent items: `/opds/v1.2/libraries/{libraryID}/recent`
       - Search: `/opds/v1.2/libraries/{libraryID}/search?q={searchTerms}`
     - Directly lists the first 20 recent items.
   - **All Items Feed (`/opds/v1.2/libraries/{libraryID}/all`)**:
     - Lists all items in the library, supports paginated navigation using `?page=X`.
   - **Recent Items Feed (`/opds/v1.2/libraries/{libraryID}/recent`)**:
     - Lists library items sorted by `addedAt` descending.
   - **Search Feed (`/opds/v1.2/libraries/{libraryID}/search`)**:
     - Performs a query filter on library items matching query string `?q=xxx`.

3. **XML/Atom Feed Translation**
   - Format each library item entry as Atom XML containing:
     - Title, Subtitle, description content.
     - Author and Narrator details.
     - Cover image link (`/api/items/{itemID}/cover`).
     - Acquisition/Download link (`/api/items/{itemID}/download`) with the correct MIME type based on format (e.g. `application/epub+zip` for EPUB ebooks, or `application/zip` for zipped audiobook folders).

4. **Integration & Routing**
   - Register the `/opds/` and `/opds` routes in `internal/handlers/routes.go`.
   - Enforce authentication via the updated `AuthMiddleware`.

5. **Frontend Web UI Integration**
   - In `frontend/js/settings.js`, add a dedicated section in Server/General settings containing a read-only input field showing the absolute OPDS URL (e.g., `http://<host>:<port>/opds`) with a copy button.

6. **Verification & Testing**
   - Create unit tests in `internal/handlers/opds_test.go` checking basic auth logic, OPDS XML structure, navigation links, and correct error handling.
   - Verify all tests compile and pass.
