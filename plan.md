# Implementation Plan - Settings Modals Scroll Containment

1. Identify Settings modals that lack proper viewport scaling constraints and overflow scroll containment. These are:
   - `showLibraryModal(lib)` in `frontend/js/settings.js`
   - `triggerCreateNotificationModal(allSettings, onSaveSuccess)` in `frontend/js/settings.js`
   - `triggerEreaderDeviceModal(device, devices, users, settings)` in `frontend/js/settings.js`

2. Refactor the wrapper element in these modals to support max height and flex-col layout:
   - Add classes `flex flex-col max-h-[90vh]` to the main modal card wrapper.
   - Separate the modal header and footer from the form fields.
   - Place all input fields inside a container class `flex-grow overflow-y-auto pr-1 no-scroll space-y-4` to handle scroll containment on narrow screens.

3. Verify that these modals compile and display correctly.

4. Run tests to confirm zero regressions.
