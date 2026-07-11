# Plan: Enhance Custom Metadata Providers (Migration & Tests)

## Objective
Enhance the Custom Metadata Providers feature to ensure database migrations handle upgrades for existing servers correctly, write unit tests for the metadata settings endpoints, and write E2E tests for the feature.

## Tasks

### 1. Database Migration (Go)
- Update `internal/db/db.go` within `migrateDatabase()` to check if the `customMetadataProviders` table exists in the SQLite database.
- If it does not exist (e.g. for upgraded databases), create it with the exact schema matching `bootstrapSchema()`:
  ```sql
  CREATE TABLE IF NOT EXISTS customMetadataProviders (
      id TEXT PRIMARY KEY,
      name TEXT,
      mediaType TEXT,
      url TEXT,
      authHeaderValue TEXT,
      extraData TEXT,
      createdAt INTEGER,
      updatedAt INTEGER
  )
  ```

### 2. Backend Handlers Unit Tests
- Create `internal/handlers/settings_metadata_test.go`.
- Write tests for all endpoints:
  - `GET /api/search/providers` (both default and registered custom providers).
  - `GET /api/custom-metadata-providers` (retrieving all custom providers).
  - `POST /api/custom-metadata-providers` (creating a new custom provider with name, url, mediaType, optional auth header).
  - `DELETE /api/custom-metadata-providers/{id}` (deleting a custom provider, verifying library fallback to defaults).

### 3. End-to-End (E2E) Test
- Create `e2e/f20_custom_metadata_provider_test.go`.
- Implement tests to verify the flow using the HTTP endpoints:
  - Add a custom provider.
  - Verify it is listed in active metadata providers for books/podcasts.
  - Perform a search that routes to the custom provider (mocked or verified).
  - Clean up/delete the custom provider.

## Verification
- Run backend unit tests: `go test ./internal/db/...` and `go test ./internal/handlers/...`.
- Run E2E tests: `go test ./e2e/...`.
