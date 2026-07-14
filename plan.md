# Plan: Auditing and Hardening Library & Item Access Control Boundaries

We will verify and enforce library-level and item-level access controls across all library sub-routes to prevent unauthorized cross-library data leakage.

## Modified Files
1. `internal/handlers/authors_series.go`
   - Retrieve `UserSession` and call `CanAccessLibrary` in `handleGetLibraryAuthors`, `handleGetLibrarySeries`, and `handleGetLibrarySeriesByID`.
   - Retrieve `UserSession` in `handleGetLibraryItemByID` and verify library access, explicit content limits, and tag exclusions/inclusion lists.
2. `internal/handlers/narrators.go`
   - Retrieve `UserSession` and verify library access in `handleGetLibraryNarrators`.
3. `internal/handlers/playlist_handlers.go`
   - Retrieve `UserSession` and verify library access in `handleGetLibraryPlaylists`, `handleGetLibraryCollections`, and `handleGetLibraryOPML`.
4. `internal/handlers/security_adversarial_test.go`
   - Add unit tests verifying that unauthorized users are rejected with `403 Forbidden` for all of these endpoints.
