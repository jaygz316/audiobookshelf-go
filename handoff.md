# Handoff: Audiobookshelf Go Port

## Targeted Task & Accomplishments
- **Target Task**: Playlist Reordering & Public Share Links Customizer
- **Accomplishments**:
  - Implemented **Drag-and-Drop Playlist Reordering** in `frontend/js/playlists.js` using HTML5 drag-and-drop APIs. Replaced manual Up/Down buttons with an intuitive grab handle (`drag_handle`) that updates item lists in real-time.
  - Enhanced **Public Share Links Customizer** with advanced capabilities:
    - **Custom Expiration Date/Time**: Added a `datetime-local` input field matching expiration rules.
    - **Password Protection**: Visual toggles to password-protect shared media.
    - **Maximum Download Limits**: Enforced `MaxDownloads` limit check in the backend `/api/s/{slug}/download` handler and incremented `downloadsCount` on each successful request.
    - **Embeddable Web Player Layout**: Dynamically toggles a premium visual compact card layout (hiding metadata descriptions and lists) when the embeddable flag is true.
  - Updated SQLite database schema and backend models in `internal/share/share.go` to support new attributes (`maxDownloads`, `downloadsCount`, and `embeddable`).
  - Successfully verified compile build and unit/integration tests (`go test ./...`). All backend tests pass.

## Outstanding Work / Next Gaps
- **Podcast Subscriptions & Episode Downloader**: Subscription portal (iTunes / PodcastIndex APIs), downloader queue UI, and subscription cleanup policies.

## Next Steps
- Implement iTunes/PodcastIndex subscription search and subscribe endpoints on the backend.
