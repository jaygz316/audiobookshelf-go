# Handoff: Audiobookshelf Go Port

## Targeted Task & Accomplishments
- **UI Cache Purging Features**:
  - Implemented the "Purge All Cache" and "Purge Items Cache" buttons under a new "Troubleshooting / Cache Tools" card in the Server Settings UI ([settings.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/settings.js)).
  - Added matching API endpoints `/api/cache/purge-all` and `/api/cache/purge-items` in the backend ([routes.go](file:///home/jay/projects/audiobookshelf-go/internal/handlers/routes.go)) that perform authenticated folder deletion (admin/root required).
- **Filesystem Security Audit & Symlink Traversal Patches**:
  - Identified a path traversal vulnerability where symbolic links targeting files/directories outside the library folders bypassed `IsSafeFilePath` because `filepath.Abs` does not resolve symlinks.
  - Patched `IsSafeFilePath` in [utils.go](file:///home/jay/projects/audiobookshelf-go/internal/utils/utils.go) to resolve symbolic links using `filepath.EvalSymlinks`, falling back gracefully to absolute path checks when targets do not yet exist.
  - Patched `streamDirAsZip` in [library_handlers.go](file:///home/jay/projects/audiobookshelf-go/internal/handlers/library_handlers.go) to explicitly skip symbolic links, preventing zip-based traversal downloads.
  - Added `TestIsSafeFilePath_SymlinkTraversal` to [path_traversal_adversarial_test.go](file:///home/jay/projects/audiobookshelf-go/internal/handlers/path_traversal_adversarial_test.go) to continuously verify symlink traversal is successfully blocked.

## Verification
- Built and ran the Go backend with `go build`, and confirmed that all unit/integration tests pass (`go test ./...` is 100% green).
