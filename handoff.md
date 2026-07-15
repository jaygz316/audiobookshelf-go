# Handoff: Audiobookshelf Go Port — UX Audit

## Current Screen Under Audit
- **Screen**: Login & Initial Setup
- **Status**: ✅ Complete

## What Was Fixed This Run
- **Visuals**:
  - Matched the Login card size, dark background, borders, and margins (`w-full max-w-md bg-bg rounded-md shadow-lg border border-white/5 p-4 flex flex-col`) to the original Nuxt component (`login.vue`).
  - Added the absolute top-left branding header (with square logo `icon.svg` and `audiobookshelf` label) to both the login and setup screens.
  - Replaced the inner-card logo on the login card with the original centered "Login" text and a horizontal divider line (`bg-white/10`).
  - Replaced the inner-card logo on the setup wizard card with "Initial Server Setup" and a horizontal divider line.
  - Restyled all input text and password fields to match `TextInput.vue`: `bg-primary text-gray-200 focus:bg-bg focus:outline-none border border-gray-600 focus:border-gray-300 rounded-sm px-3 py-2 text-sm`.
  - Added two disabled/read-only inputs for "Config Path" and "Metadata Path" under "Directory Paths" section in the Initial Setup wizard.
- **Functional & Interactions**:
  - Passed the `status` object to `showSetupScreen(status)` upon initialization and populated the new Config Path and Metadata Path fields.
  - Wired the Login submit button to show "Checking..." and disable itself during submission, and restore itself if failed.
  - Aligned both Login and Setup submit buttons visually to the bottom-right of the forms using `justify-end` and style matching `Btn.vue`.

## Remaining Issues on This Screen
- None. Login and Initial Setup screen parity has been fully achieved.

## Next Screen in Queue
- Home / Dashboard (Wooden bookshelf texture/background, shelf rows, book cover cards, card reflections, hover shadows, and shelf sizing slider in bottom-right).

## Buttons/Controls Verified Working This Run
- **Sign In / Login Submit Button**: Submits credentials, transitions to "Checking...", disables input, and processes authorization.
- **Initial Setup Submit Button**: Validates passwords, transitions to "Initializing...", disables inputs, sends root user creation, and signs user in automatically.
- **OIDC Button**: Dynamically rendered if openid is enabled in methods, uses custom display labels, and handles callback auth redirection correctly.

## Buttons/Controls Known Broken
- None.
