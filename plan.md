# Implementation Plan: Per-Item Access Restrictions

We will implement complete user interface controls for per-user tag-based access restrictions (hiding or showing specific tags/genres) and resolve related parameter-binding bugs in the Go backend.

## Proposed Changes

1. **Backend Bug Fixes & Refactoring**
   - **Fix SQLite Bindings**: In `getUserPermissionWhere` in [db_queries.go](file:///home/jay/projects/audiobookshelf-go/internal/db/db_queries.go), fix `args = append(args, args...) // duplicate for bindings` which causes a parameter binding mismatch when checking items with tag restrictions. It should simply return `args` matching the `placeholders` count.
   - **Update User Handler PATCH endpoint**: In [users.go](file:///home/jay/projects/audiobookshelf-go/internal/handlers/users.go), update the JSON structure of `PATCH /api/users/{id}` to correctly parse and update `accessAllTags`, `itemTagsSelected`, and `selectedTagsNotAccessible` (handling empty selections `[]`). Allow passing these properties inside the `permissions` map for consistency with standard API structures.

2. **Frontend UI Integration**
   - In [settings.js](file:///home/jay/projects/audiobookshelf-go/frontend/js/settings.js):
     - Fetch the current master tags using `request('GET', '/api/tags')`.
     - In the user modal (`triggerUserModal`), render:
       - **Access All Tags** checkbox (`id="perm-all-tags"`).
       - **Tag Restriction Mode** dropdown (`id="perm-tags-not-accessible"`):
         - "Allow Only Selected Tags" (maps to `selectedTagsNotAccessible = false`)
         - "Block Selected Tags" (maps to `selectedTagsNotAccessible = true`)
       - **Selected Tags** checkbox list (`id="tag-filter-container"`), dynamically rendered and filtered when "Access All Tags" is unchecked.
     - Ensure the payload for both `POST /api/users` and `PATCH /api/users/{id}` properly parses and sends the user's tag-based settings to the backend.

3. **E2E Integration & Unit Testing**
   - Add unit tests or integration tests targeting the SQLite queries containing tag restrictions to ensure no "wrong number of SQL variables" error occurs.
   - Verify user-specific items filtering (hiding/revealing tags) works end-to-end.
