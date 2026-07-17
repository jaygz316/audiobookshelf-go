# Plan: Refine Playlists, Collections, and Stats UI

1. **Playlist Cards Aspect Ratio**:
   - In `frontend/js/playlists.js`, change the cover container's `aspect-ratio` from `2/3` to `1/1` (square) to align with original Audiobookshelf playlist cards.
2. **Collection Cards Aspect Ratio**:
   - In `frontend/js/collections.js`, change the cover container's `aspect-ratio` from `2/3` to `1/1` (square).
3. **Listening Stats Chart Responsiveness**:
   - In `frontend/js/stats.js`, migrate absolute HTML positioning of Y-axis grids, labels, tooltips, and day labels directly into the SVG coordinates so they scale responsively together.
   - Set the container to a responsive width and height (`w-full max-w-[384px] aspect-[4/3]`) and use `viewBox` on the SVG so the layout scales cleanly across all device widths.
4. **Compile and Verify**:
   - Build the backend using `go build` / `go run run.go build` and run unit tests to verify no compilation errors.
