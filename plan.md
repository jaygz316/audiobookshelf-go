# Plan: Audit Library Settings UI Parity

1.  **Objective**: Align Settings > Libraries list UI with original Audiobookshelf premium aesthetic.
2.  **Tasks**:
    *   Review `renderLibrariesTab` in `frontend/js/settings.js`.
    *   Verify CSS usage for `library-row`.
    *   Apply premium styling consistent with global theme variables in `variables.css`.
    *   Improve hover effects, border highlights, and padding to match reference.
    *   Ensure consistency with `components.css` classes.
3.  **Plan**:
    *   Check `components.css` for existing library-row style definitions.
    *   Modify `renderLibrariesTab` HTML if needed to match the required structure.
    *   Add specific CSS classes to `components.css` to match the "wooden shelf" / "premium" look if missing.
    *   Verify and commit.
