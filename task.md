# Audiobookshelf Go: Task List & Feature Parity Checklist

This document tracks the tasks and missing features of the `audiobookshelf-go` project compared to the original Audiobookshelf web client.

## Completed Tasks

### Smart Collections Implementation
- [x] Database Schema Migration: Add `isSmart` and `rules` columns to the `collections` table.
- [x] Playlist Manager & Backend Logic: Parse and query SQLite dynamically based on rules.
- [x] API Handlers Update: Load and update collections with JSON payloads.
- [x] Frontend UI Updates: Create rule inputs and hide manual actions for smart collections.
- [x] Verification: Integration/unit tests for smart collection rules.

### UI/UX Visual Alignment & Layout Symmetry
- [x] Bookshelf shelf size adjuster slider (`- 120 +`) in bottom-right corner.
- [x] Bookshelf style switcher (wooden shelf graphic vs. flat grid vs. detail list) with custom shadow/reflection styling.
- [x] Series View overlapping cascading/fanned cards with book count badge.
- [x] Top header bar with logo, brand label, library dropdown, cast icon, server stats/activity icon, upload icon, settings gear, and user profile pill badge with username and initials.
- [x] Siderail left navigation layout and icon symmetry.

## Missing UI Features (Feature Parity Roadmap)

### 1. Playback & Player Interface (Web Client)
- [ ] **Interactive Visual Waveforms**: Generate and render dynamic SVGs/canvas waveforms in the player bar for seeking.
- [x] **Advanced Playback Speed Controls**: Add a fine-tuned slider/preset menu (0.5x to 3.0x in 0.05x increments) and speed persistence (global vs. per-book).
- [x] **Volume Boost & Equalizer Controls**: Implement volume booster slider and preset EQ controls.
- [x] **Comprehensive Sleep Timer Settings**:
  - [x] Sleep timer duration selector (minutes or end-of-chapter).
  - [x] Shake-to-extend toggle and sensitivity control.
  - [x] Gradual audio fade-out timer customization.
- [x] **Play History Panel**: Track and render detailed timelines of previous listening sessions, showing device name, duration, and exact timestamps.
- [ ] **Active Playback Queue Manager**: UI to view, append, reorder (via drag handles), and clear current tracks or books queue.
- [x] **Bookmarks Manager Panel**: Bookmark creation with custom text notes, color tags, and export/import bookmarks.

### 2. E-Book Reader UI
- [x] **Flow vs. Paginated Layouts**: Add toggle for continuous vertical scroll vs. page-by-page view.
- [x] **Reader Typography & Themes Panel**: Custom settings for font size, line spacing, margins, font family (including OpenDyslexic), and color profiles (sepia, dark, warm, light).
- [x] **Reader Bookmarks & Highlights Side-Panel**: View, navigate, and search within user-saved highlights and notes in the EPUB.
- [x] **Text-To-Speech (TTS) Controls**: Built-in browser-based screen reader controls for EPUB reading.
- [ ] **PDF Reader Enhancements**: Add page thumbnails side rail, search page index, and zoom in/out controls.

### 3. Library Sorting, Filtering & Presets
- [ ] **Custom Search Presets**: Save and name custom combinations of filters/sort options as quick-access tabs on the main navigation.
- [x] **Comprehensive Grid Filters**: Dropdown selection filters for Publisher, Release Year, Narrator, Series, Progress State (unstarted, in-progress, completed), Duration (under 1h, 1-5h, etc.), and Folder Path.
- [ ] **Grid Layout Sizing & Bookshelf Customizer**:
  - [x] Bookshelf shelf size adjuster slider (`- 120 +`).
  - [x] Bookshelf style switcher (wooden shelf graphic vs. flat grid vs. detail list).
  - [ ] Column customization for the detail list view.

### 4. Metadata Management & Interactive Editors
- [ ] **Visual Match Dialog (Diff Viewer)**: Compare side-by-side search results from metadata providers (Audible, Open Library, Google Books, etc.) before applying changes.
- [ ] **Granular Field Lock System**: Checkboxes next to individual metadata fields (Title, Author, Narrator, Series, Year, Genre) to prevent auto-scans from overwriting them.
- [ ] **Chapter Editor Suite**:
  - [ ] Dynamic chapter visual waveform alignment.
  - [ ] Manual chapter actions: Add, delete, shift start/end timestamps, rename.
  - [ ] Automatic chapter extraction from audio track markers or lookup via external APIs (Audnexus).
- [ ] **Cover Art Editing Canvas**: Crop tool, image color picker, and cover search results gallery.
- [ ] **Batch Metadata Editor**: Multi-select items in the library grid to edit genres, tags, authors, narrators, series, publishers, and release years in bulk.

### 5. Podcast Subscriptions & Episode Downloader
- [ ] **Podcast Search & Subscription Portal**: In-app search for subscribing to feeds using iTunes and PodcastIndex APIs.
- [ ] **Podcast Download Queue UI**: View active downloads, download speeds, pending queue, retry failed downloads, and pause/resume buttons.
- [ ] **Subscription Cleanup Policies**: Dropdown settings per podcast for episode retention limits, automatic deletion of played episodes, and check schedule intervals.

### 6. Server Administration & Permissions
- [ ] **Granular User Permissions Manager**: Checkboxes for editing individual permissions:
  - [ ] Access specific libraries.
  - [ ] Upload files / Delete media.
  - [ ] Edit metadata / Force library scans.
  - [ ] Access RSS feeds / Create public shares.
- [ ] **Active Session List**: View current active tokens, login timestamps, device operating system/browser, IP address, and single-click "Revoke Session" buttons.
- [ ] **API Keys Management Tab**: Create API keys with descriptions, view masked key strings, copy key, and revoke keys.

### 7. Backups, Notifications & Settings Tabs
- [x] **SMTP & Kindle Configuration**: SMTP server connection tester, and per-user Kindle email addresses manager.
- [x] **Notification Integrations Panel**: Form fields to configure Discord, Matrix, Gotify, Telegram, Slack, or Webhook notifications.
- [x] **Backup Operations UI**: Create scheduled backup crons, lists of backup ZIPs with Download, Restore, and Delete actions.
- [x] **Real-time Server Console / Log Stream**: Interactive terminal panel in administrative settings showing streaming log output via Socket.io.

### 8. Playlists & Public Sharing
- [ ] **Drag-and-Drop Playlist Reordering**: Sort playlist tracks by dragging handle icons.
- [ ] **Public Share Links Customizer**:
  - [ ] Custom expiration dates/times.
  - [ ] Password protection.
  - [ ] Maximum download limits.
  - [ ] Embeddable web player configuration.
- [x] **Smart Collection Rules Builder**: Multi-clause rules editor UI for nested dynamic logic (e.g., Tag = 'sci-fi' AND Author = 'Asimov').
