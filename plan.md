# Build Plan - Comprehensive Grid Filters

1. Modify `internal/db/db_queries.go` in `getFilterWhere` function to support filtering by:
   - `decades.<decade>` (published decade)
   - `years.<year>` (published year)
   - `duration.<range>` (duration buckets)
   - `folder.<folderId>` (folder ID)
2. Update `frontend/js/dashboard.js` to correctly load default sort, sort order, and filters from `localStorage` inside `loadDashboard` if not explicitly passed.
3. Enhance the custom Filter Dropdown UI in `frontend/index.html` and `frontend/js/app.js` to fetch library filter data (`GET /api/libraries/{id}/filterdata`) and display a beautiful categorical dropdown layout.
4. Verify the changes using `go build && go test ./...`.
