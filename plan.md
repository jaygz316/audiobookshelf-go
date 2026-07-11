# Plan: Storage Path Isolation

## Objective
Implement Storage Path Isolation feature, ensuring that metadata (e.g. `metadata.json`) and covers (e.g. `cover.jpg`) are saved to the centralized metadata directory rather than local ebook/audiobook directories if `MetadataMarkdownWithItem` or `MetadataCoverWithItem` are disabled.

## Status
- **State**: Completed and verified.
- **Verification**: Fully covered by `TestStoragePathIsolation` in `internal/scanner/scanner_test.go` and comprehensive package/E2E test suite.

## Tasks & Changes

### 1. Centralized Metadata Path Injection
- In `internal/handlers/routes.go`, captured the global `cfg.MetadataPath` into a package-level variable and exported/passed it to the scanner package as `scanner.MetadataPath` during application initialization.

### 2. Scanner Isolation Compliance
- In `internal/scanner/scanner.go`:
  - Imported `idb "audiobookshelf/internal/db"` to retrieve dynamic server settings.
  - Declared package-level `var MetadataPath string`.
  - Updated `parseMetadataForGroup` signature and implementation to accept `itemID string` to support item-scoped folder creation.
  - Retrieved `metadataCoverWithItem` server settings. If disabled, saved extracted covers under `/metadata/items/{itemID}/cover.jpg` instead of local media directory.
  - Updated scanner call sites in `scanNewLibraryItem` and `scanExistingLibraryItem` to propagate `itemID`.

### 3. API Handlers Compliance
- In `internal/handlers/authors_series.go`:
  - Updated single metadata updates to check `MetadataMarkdownWithItem` server settings. If disabled, read and wrote metadata sidecars at `/metadata/items/{itemID}/metadata.json` instead of local book directory.
- In `internal/handlers/batch_edit.go`:
  - Updated batch metadata updates to check `MetadataMarkdownWithItem`. If disabled, stored metadata sidecars under `/metadata/items/{itemID}/metadata.json`.

### 4. Verification
- Created and executed `TestStoragePathIsolation` inside `internal/scanner/scanner_test.go` verifying compliance with both folder-based and isolated/centralized storage scenarios.
