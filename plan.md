# Plan: Audiobookshelf Go Port - UI Header Count Regression & Formatting Fixes

1. **Header Count Regression**:
   - Change `showBookCount` variable in `frontend/js/app.js` to be true on `/series`, `/authors`, `/collections`, `/playlists`, and `/narrators` views as well, not just `/library`. This ensures the counts computed inside their respective JS views are not hidden by routing transitions.

2. **Rebuild & Verification**:
   - Compile WebAssembly and build: `go run run.go build`
   - Run tests: `go run run.go test`
   - Vet: `go run run.go vet`
