# Plan: Granular User Permissions Support

## Goals:
1. Stage and commit previous EPUB, PDF reader, and queue manager changes.
2. Extend user permissions models on backend (`UserPermissionsDetailed`, `userPermissions`, and `core.UserSession`) to include `upload`, `delete`, `update`, `accessRss`, and `createShares`.
3. Add form fields (toggles/checkboxes) in the User Modal in `frontend/js/settings.js` to manage these granular permissions.
4. Update API permission extraction in the backend and enforce permissions on upload, delete, update, RSS, and public share API endpoints.
5. Run tests to verify backend compilation and correct handlers enforcement.

## Files to Modify:
- `internal/core/core.go`: Update `UserSession` struct.
- `internal/db/db.go`: Update `userPermissions` struct and `ParsePermissions` function.
- `internal/db/users.go`: Update `UserPermissionsDetailed` struct.
- `internal/handlers/users.go`: Parse the new permissions in create/update handlers.
- `internal/handlers/library_handlers.go`: Enforce update/delete/upload permissions.
- `internal/handlers/share_handlers.go`: Enforce create shares permission.
- `internal/handlers/feeds.go`: Enforce RSS permission.
- `frontend/js/settings.js`: Add checkboxes for new permissions in `triggerUserModal`.
- `task.md`: Mark granular permissions task as checked.
