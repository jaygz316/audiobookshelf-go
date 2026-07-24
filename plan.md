# Plan: Podcast UI Refinement

1.  **Objective**: Improve podcast episode list item styling to better match the high-fidelity UI of the original Audiobookshelf.
2.  **Target File**: `/home/jay/projects/audiobookshelf-go/frontend/js/podcasts.js`
3.  **Actions**:
    - Update list item container styling (`flex items-center justify-between p-3`) to incorporate slightly more depth or hover state transitions.
    - Enhance button styling (`Play`/`Download`) to ensure they perfectly align with the design system (rounded corners, specific background colors `#2c2c2c` vs `accent`).
    - Audit `coverHtml` generation to ensure consistent dimensions and border styling.
4. **Verification**: After applying, build and run to ensure no regressions and verify visual improvement.
