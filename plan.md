# Implementation Plan: E-Book Send to Device & SMTP/E-Reader Settings

## Objective
Provide frontend UI support for configuring SMTP email settings, managing e-reader devices, and sending e-books to configured devices.

## Backend Changes (Go)
1. **New Route**:
   - Register endpoint `GET /api/emails/devices` in `internal/handlers/routes.go`.
2. **Device Listing Handler**:
   - Implement `handleGetAvailableDevices(db *sql.DB)` in `internal/handlers/email_handlers.go`.
   - Retrieve email settings from SQLite (`email-settings` row in `settings` table).
   - Filter devices based on current user's session role (`root`/`admin` gets all devices, others only get devices where `availabilityOption == "allUsers"` or the user is explicitly in the `users` array).
   - Return a sanitized JSON array of device names to the frontend.

## Frontend Changes (Vue/Static HTML/JS)
1. **Settings Page Tab**:
   - Edit `frontend/js/settings.js` to add an "Email (E-Readers)" tab under the settings page navigation.
   - Implement `renderEmailsTab()` to load and render:
     - **SMTP Server Configuration Form**: fields for `host`, `port`, `secure`, `rejectUnauthorized`, `user`, `pass` (sanitized with dots/asterisks, only update if changed), `testAddress`, and `fromAddress`. Add "Save Settings" and "Send Test Email" buttons.
     - **E-Reader Devices Table**: displays device Name, Email address, Availability setting, and Action buttons (Edit/Delete).
     - **Add/Edit E-Reader Device Modal**: handles setting Name, Email, Availability option (`allUsers`, `adminOrUp`, `specificUsers`), and selecting specific users (if `specificUsers` is chosen).
2. **Item Details Page "Send to Device" Action**:
   - Edit `frontend/js/itemDetails.js` to add a "Send to Device" button next to "Read Book" if the item has an e-book (`hasEbook === true`).
   - Clicking this button fetches available devices for the current user from `/api/emails/devices`.
   - If none are configured or available, show a helpful alert/toast.
   - If available, show a clean dropdown modal to select the target device, and trigger a `POST /api/emails/send-ebook-to-device` request with `libraryItemId` and `deviceName`.

## Verification & Testing
1. **Go Unit Tests**:
   - Add unit tests for the new `/api/emails/devices` endpoint in `internal/handlers/email_handlers_test.go`.
2. **E2E/Integration Verifications**:
   - Run existing unit tests to verify no regressions.
