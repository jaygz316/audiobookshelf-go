# Plan: Concurrent Library Folder Scanner Optimization

## Objective
Optimize the library scanner to process file metadata extraction (via ffprobe and tag parsing) and directory scanning concurrently, significantly reducing scan times for libraries with multiple files/items.

## Proposed Changes
1. **Global Concurrency Limiting**:
   - Introduce a package-level `probeSemaphore` in `internal/scanner/scanner.go` using `init()` to bound the maximum number of concurrent `ffprobe` subprocesses (between 4 and 8, based on `runtime.NumCPU()`).
2. **Parallelize Audio File Parsing inside parseMetadataForGroup**:
   - Process the audio files list inside `parseMetadataForGroup` concurrently. Use a `sync.WaitGroup` and store parsed objects by index. Apply fallbacks and post-processing sequentially to preserve the original order and logic.
3. **Refactor scanNewLibraryItem and scanExistingLibraryItem**:
   - Update their signatures to accept pre-parsed `*GroupMetadata` to decouple DB transactions from CPU/IO-heavy parsing.
4. **Parallelize Library Item Processing inside ScanLibrary**:
   - Split the library item processing loop into a read/check phase, a concurrent parsing phase (using a worker pool), and a sequential DB write phase.
