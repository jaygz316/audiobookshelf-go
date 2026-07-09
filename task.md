# Audiobookshelf Go Rewrite: Settings Tasks

## R1. General Settings
- [x] Store covers with item (`metadataCoverWithItem`)
- [x] Store metadata with item (`metadataMarkdownWithItem`)
- [x] Ignore prefixes when sorting (`sortingIgnorePrefix`)

## R2. Scanner Settings
- [x] Persistent scanner settings (`scannerParseSubtitles`, `scannerFindCovers`, `scannerCoverProvider`, `scannerPreferMatchedMetadata`)
- [x] Automatically watch libraries for changes (`watchLibraryChanges`)

## R3. Web Client Settings
- [x] Chromecast support (`chromecastEnabled`)
- [x] Allow embedding in an iframe (`allowIframe`)

## R4. Display Settings
- [x] Home page and library bookshelf view options (`homePageBookshelfView`, `libraryBookshelfView`)
- [x] Date/Time Format & Default Server Language dropdowns (`dateFormat`, `timeFormat`, `language`)

## R5. Security Settings
- [x] Allowed CORS Origins (`allowedCorsOrigins`)

## Acceptance Criteria
- [x] UI layout and database persistence via GET/PATCH `/api/settings`
- [x] Dynamic CORS handling
- [x] Iframe block handling
- [x] File watcher directory mapping adjustments
- [x] Metadata cover path saving destination adjustments
- [x] All backend unit and E2E integration tests compiling and passing
