# Plan - Enhanced Player Settings (Playback Speed Persistence & Volume Boost)

1. **HTML Modifications (`frontend/index.html`)**:
   - Add a player settings button (`#player-settings-btn`) to the center controls group next to the sleep timer.
   
2. **Web Client Logic (`frontend/js/player.js`)**:
   - Introduce an `AudioContext` and a `GainNode` for real-time volume boosting.
   - Implement `initVolumeBoost()` to route `<audio>` output through the GainNode when boost is > 1.0 (or always route it once initialized).
   - Implement playback speed persistence:
     - Load global settings (`abs-speed-global` and `abs-speed-remember-per-book`).
     - Load/save per-book playback speed (`abs-speed-book-<id>`).
     - Apply correct speed on playback start.
   - Implement Player Settings modal (`triggerPlayerSettingsModal()`) containing:
     - Remember playback speed per book (checkbox).
     - Global default playback speed (select).
     - Volume Boost multiplier (slider/select, e.g. 1.0x to 3.0x).
   - Add event listener for `#player-settings-btn` to open the modal.
   - Ensure the quick speed select (`#player-speed`) on the player bar correctly updates the persisted setting (either per-book or global).

3. **Backend & Test Verification**:
   - Run `go build` and `go test` to ensure zero compilation or build regressions.
