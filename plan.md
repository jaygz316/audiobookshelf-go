# Plan: Bookshelf View & Cards Alignment

## Objective
Implement `-webkit-box-reflect` and refine hover shadows for bookshelf cards in `frontend/css/layout.css` to match the original project's premium feel.

## Plan
1. Edit `frontend/css/layout.css`.
2. Locate `.library-shelf-grid > .group .book-cover-wrapper`.
3. Add `-webkit-box-reflect` rule with reflection offset, gradient, and alpha settings.
4. Refine `.library-shelf-grid > .group:hover .book-cover-wrapper` box-shadows to add a more pronounced glow/depth.
5. Rebuild the frontend assets (since CSS changes require rebuilding the embedded assets).
6. Verify visual changes with a local build.
