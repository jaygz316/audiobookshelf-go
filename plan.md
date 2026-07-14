# Plan: EPUB Reader Enhancements

## Goals:
1. **Flow vs. Paginated Layouts**: Add settings toggle and logic to support continuous vertical scroll (`scrolled-doc`) and page-by-page rendering (`paginated`).
2. **Warm Theme**: Add color profile `warm` (background: `#fbf0e3`, text: `#5c4033`) in theme selection.
3. **Reader Bookmarks & Highlights Side-Panel**: View, navigate, and search user highlights and notes. Support selecting text in the reader to create a highlight with a note.
4. **TTS Controls**: Built-in screen reader using Web Speech API to read chapter text aloud.

## Files to Modify:
- `frontend/js/reader.js`: Implement settings popover updates, EpubJS render changes for scrolled flow, highlight selection listener, side-panel render/actions, and speech synthesis logic.
