# Handoff: Audiobookshelf Go Port

## Targeted Task & Accomplishments
- **Target Task**: Active Playback Queue Manager Drag-and-Drop Reordering.
- **Accomplishments**:
  - Defined module-level tracking variable `draggedQueueIndex` in `frontend/js/player.js` to manage dragging states browser-independently.
  - Enhanced the Playback Queue dialog items to be draggable using the HTML5 drag-and-drop APIs.
  - Added visual drag handles (`drag_handle` material symbol) with grab/grabbing cursors to each queue row.
  - Linked drag-and-drop actions (`dragstart`, `dragend`, `dragover`, `dragenter`, `dragleave`, `drop`) to call `reorderQueue` and automatically refresh the queue modal reactively.
  - Formatted, compiled, and tested the Go backend cleanly.

## Outstanding Work / Next Gaps
- **Granular Field Lock System (UI & Backend)**: Checkboxes in the metadata modal next to fields (Title, Author, Narrator, Series, etc.) to prevent overwrite during scan.
- **Chapter Editor Suite**: Manual adjustment, automatic extraction, and lookup of chapters.
- **Batch Metadata Editor**: Bulk updates of metadata on multi-selected library items.

## Next Steps
- Implement the **Granular Field Lock System** in both the frontend metadata dialog (`frontend/js/itemDetails.js`) and backend database/API.
