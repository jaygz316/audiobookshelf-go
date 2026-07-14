# Plan: Granular Field Lock System

Implement lock checkboxes/icons in the item edit details modal (`frontend/js/itemDetails.js`) for the metadata fields to protect them from being overwritten during folder/metadata scans.

1. **Identify Lockable Fields**: title, subtitle, authors, narrators, series, description, publisher, publishedYear, publishedDate, isbn, asin, language, genres, tags.
2. **UI Implementation**:
   - Update `triggerEditItemDetailsModal` in `frontend/js/itemDetails.js` to render clickable material symbol locks (`lock` or `lock_open`) next to the field labels.
   - Maintain a `currentLockedFields` Set initialized from `metadata.lockedFields` or `lockedFields`.
   - Implement click event delegation for `.metadata-lock-btn` to toggle the lock state, updating the icon and CSS classes in real-time.
   - Collect `lockedFields` array from the Set and submit it in the payload to `/api/items/${item.id}` during update.
3. **Verify backend compatibility**: Ensure tests build and run cleanly.
