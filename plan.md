# Plan - WebSockets/Socket.io Presence Sync and Player Event Handling Alignment

Align the Audiobookshelf Go port's socket presence and player event UI states with the original project.

## Implementation Details

1. **CSS Enhancements** (`frontend/css/components.css`):
   - Add `.playing-visualizer` (animated Equalizer bars) styling and `@keyframes bounce-equalizer`.
   - Add `.presence-badges-container` and `.presence-avatar-badge` styles with tooltips for active listeners presence mapping.

2. **Playback Event Dispatcher** (`frontend/js/player.js`):
   - Inside the audio 'play', 'pause', and 'ended' listeners, dispatch custom DOM events (`playback-state-changed`) containing the current item ID and playing status.

3. **Active Session websocket Tracking** (`frontend/js/socket.js`):
   - Maintain a global reactive store (`window.activePlaybackSessions`) of all active playback sessions on the server.
   - Update it on WebSocket `init`, `playback_session_added`, `playback_session_updated`, and `playback_session_removed` events.
   - Dispatch a `'presence-updated'` custom event to trigger card refresh.

4. **Bookshelf Card Rendering & Real-time Update Bindings** (`frontend/js/dashboard.js`):
   - Update `createCard` to check current local playing state and other users' session mapping to render:
     - The interactive visualizer bar overlay.
     - Toggle play/pause buttons directly from the card.
     - Display initials avatars with tooltips representing other users' active presence.
   - Register dynamic event listeners for `'playback-state-changed'` and `'presence-updated'` to update existing DOM nodes directly.
