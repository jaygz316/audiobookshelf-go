# Handoff: Audiobookshelf Go Port

## Targeted Task & Accomplishments
- **Target Task**: EPUB Reader Enhancements (Layout flow toggles, custom Warm theme, bookmarks/highlights panel, TTS controls, and highlights storage with CFI)
- **Accomplishments**:
  - Implemented Flow vs. Paginated layouts toggle in the settings menu, using `localStorage` persistence and updating EpubJS rendition settings dynamically.
  - Implemented the custom "Warm" theme option (using background `#fbf0e3` and text `#5c4033`) in the themes list and updated EpubJS theme injection rules.
  - Developed the reader bookmarks & highlights side-drawer panel allowing searching and quick navigation to highlights, and integrated context-selection logic to easily create, color-code, and add notes to text highlights.
  - Added native browser-based Text-To-Speech (TTS) engine support to read chapter text aloud, complete with a clean UI play/pause button, rate adjustment slider, and a dynamic voice selector list.
  - Expanded the backend bookmark endpoint in `internal/handlers/me.go` to store `cfi` location metadata inside the SQLite bookmarks JSON object.
  - Updated the roadmap feature checklist in `task.md` to check off Flow vs. Paginated Layouts, Reader Typography & Themes Panel, Bookmarks & Highlights Side-Panel, and Text-To-Speech (TTS) Controls.

## Outstanding Work / Next Gaps
- Interactive Visual Waveforms: Generate and render dynamic SVGs/canvas waveforms in the player bar for seeking.
- Active Playback Queue Manager: UI to view, append, reorder (via drag handles), and clear the current track queue.
- PDF Reader: Add page thumbnails side rail, search page index, and zoom in/out controls.

## Next Steps
- Implement Interactive Visual Waveforms in the playback bar.
