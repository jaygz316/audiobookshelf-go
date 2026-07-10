# Implementation Plan: Multi-File Audiobook Merging

## Objective
Implement a mechanism to merge multiple separate audio files (tracks) of an audiobook into a single, standardized M4B file with custom chapter markers representing the original tracks.

## Backend Changes (Go)
1. **API Route**:
   - Register endpoint `POST /api/items/{id}/merge` in `internal/handlers/routes.go`.
2. **Merge Handler**:
   - Create `internal/handlers/merge_handler.go`.
   - Implement `handleMergeAudioFiles(db *sql.DB, metadataPath string)`:
     - Load the library item and verify it is a book.
     - Retrieve the list of audio files from the `books` table (`audioFiles` BLOB).
     - Verify the list contains at least 2 audio files.
     - Create an ffmpeg concat input text file (`concat.txt`) referencing absolute paths of all audio files.
     - Construct a new single M4B file path in the item's directory (e.g. `<Title>_merged.m4b`).
     - Run `ffmpeg -y -f concat -safe 0 -i concat.txt -c copy <output-file-path>`.
     - Read duration and codec metadata of the newly merged file.
     - Auto-generate chapters where each original track is a chapter:
       - Chapter 1: start 0, end = duration of track 1, title = track 1 filename/title
       - Chapter 2: start = sum of durations before track 2, end = start + duration of track 2, etc.
     - Update the database `books` table:
       - Set `audioFiles` to a single-element JSON array containing the new merged M4B file.
       - Set `chapters` to the auto-generated chapters array JSON.
       - Set `duration` to the total duration.
     - Optionally delete the original files to keep media folders clean, or let the user decide. For simplicity and reliability, we'll delete the original files since they are successfully merged.
     - Save updated libraryItem sizes/mtime/updatedAt in DB.
     - Return the updated library item.

## Frontend Changes (Vue/Static HTML)
1. **UI Button**:
   - In `frontend/js/itemDetails.js`, look for where the admin buttons (`details-embed-metadata-btn`) are rendered.
   - If the item has more than 1 audio file (`item.media.audioFiles && item.media.audioFiles.length > 1`), render a "Merge Audio Files" button: `details-merge-audio-btn`.
2. **Action Handler**:
   - Attach a click listener to `details-merge-audio-btn`.
   - Show a loading spinner and trigger a `POST` request to `/api/items/${item.id}/merge`.
   - On success, display a success toast/notification and refresh the item details view.

## Verification
1. **Unit Tests**:
   - Create `internal/handlers/merge_handler_test.go` to test backend merging logic using mocks.
2. **E2E/Integration Tests**:
   - Ensure the command line tool/tests run correctly.
