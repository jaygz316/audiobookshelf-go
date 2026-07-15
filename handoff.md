# Handoff: Audiobookshelf Go Port — UX Audit

## Current Screen Under Audit
- **Screen**: Add Library & Directory Picker Overlay (Priority 5)
- **Status**: ✅ Complete

## What Was Fixed This Run
- **Directory Picker Select Flow Bug**: Fixed a bug where double-clicking or navigating up in the folder picker permanently disabled the "Select Folder" button by resetting `selectedPath = currentPath` on navigation and ensuring `selectBtn` is re-enabled upon loading.
- **Directory Picker Tailwind classes alignment**: Replaced undefined/broken Tailwind classes like `border-black-350` and `bg-black-700/50` with defined classes (`border-black-300`, `bg-black-500`, `divide-black-400`).
- **Backend Folder Path Update Discarding**: Discovered and fixed a major backend bug where modifying the folder path of an existing library folder row was silently discarded by the Go backend. Added path validation/resolution in `HandleUpdateLibrary` and DB execution code in `UpdateLibrary` to correctly update changed path entries in `libraryFolders`.

## Remaining Issues on This Screen
- None.

## Next Screen in Queue
- **Priority 10 — Settings Screens: Users settings & Server settings**: Audit visual parity, permissions checkboxes, and server-wide settings toggles.

## Buttons/Controls Verified Working This Run
- **Browse Folder button in library modal**: Opens directory picker overlay.
- **Picker Navigation (double-click / Up button)**: Traverses directories and updates subfolders panel.
- **Select Folder button**: Properly copies highlighted or current directory path to input fields.
- **Update Library form submit**: Saves new and updated folder paths correctly to the DB.

## Buttons/Controls Known Broken
- None.
