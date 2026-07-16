# Plan: Settings Screens Visual Parity & Toast Alignment Audit

## Objectives
- Replace all remaining native browser `alert()` popups in `frontend/js/settings.js` with `showToast` notifications (success, warning, error) matching premium UI design.
- Add Material Symbols inline icons to settings actions and buttons across all tabs to match the aesthetic of the original Audiobookshelf client:
  - **Server Settings**: Save Server Settings (`save`), Save Prefixes (`save`), Copy OPDS (`content_copy`), Purge Cache (`delete_sweep`).
  - **Authentication**: Save Auth Settings (`save`).
  - **Backups**: Save Backup Schedule/Path (`save`), Create Backup (`backup`), Upload Backup (`upload`), Apply/Delete Backup (`check`/`delete`).
  - **Custom Providers**: Add/Save (`add`/`save`), Delete (`delete`).
  - **Upload**: Upload Files (`upload`).
  - **Users**: Unlink OIDC (`link_off`), Edit (`edit`), Delete (`delete`), Save/Cancel (`save`/`close`).
  - **API Keys**: Delete (`delete`), Create Key (`add`).
  - **Playback & Login Sessions**: Revoke/Close (`close`).
  - **Notifications**: Save Settings (`save`), Add/Edit/Delete Setup (`add`/`edit`/`delete`).
  - **Shares**: Revoke Link (`close`).
  - **Libraries**: Scan (`sync`), Save/Cancel modal (`check`/`close`).

## Verification Plan
1. Rebuild WASM and static assets.
2. Run standard tests: `go run run.go test` to verify zero regression.
