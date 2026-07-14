# Plan: Database Query Optimization

We will optimize the database layer's library deletion and update handlers by batching operations instead of running O(N) loop queries.

## Tasks
1. Edit `internal/db/db_queries.go` to update:
   - `DeleteLibrary` to delete mediaProgresses, playlistItems, and libraryItems in batch.
   - `UpdateLibrary` to delete mediaProgresses, playlistItems, and libraryItems in batch per folder deletion.
2. Run baseline benchmarks (`BenchmarkDeleteLibrary` and `BenchmarkUpdateLibrary`) to verify correctness and measure performance improvements.
3. Update `features.md` and `handoff.md`.
