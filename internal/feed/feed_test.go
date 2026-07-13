package feed

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	_ "modernc.org/sqlite"
)

var dbCounter int64

func setupTestDB(t *testing.T) *sql.DB {
	id := atomic.AddInt64(&dbCounter, 1)
	dsn := fmt.Sprintf("file:feedmemdb%d?mode=memory&cache=shared", id)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	db.SetMaxIdleConns(2)

	var schemas = []string{
		`CREATE TABLE libraries (
			id TEXT PRIMARY KEY, 
			name TEXT, 
			displayOrder INTEGER, 
			icon TEXT, 
			mediaType TEXT, 
			provider TEXT, 
			lastScan TEXT, 
			lastScanVersion TEXT, 
			settings TEXT, 
			createdAt TEXT, 
			updatedAt TEXT
		)`,
		`CREATE TABLE users (
			id TEXT PRIMARY KEY, 
			username TEXT, 
			type TEXT, 
			isActive INTEGER, 
			permissions TEXT, 
			extraData TEXT
		)`,
		`CREATE TABLE libraryItems (
			id TEXT PRIMARY KEY, 
			ino TEXT, 
			libraryId TEXT, 
			path TEXT, 
			relPath TEXT, 
			isFile INTEGER, 
			mtime TEXT, 
			ctime TEXT, 
			birthtime TEXT, 
			createdAt TEXT, 
			updatedAt TEXT, 
			isMissing INTEGER, 
			isInvalid INTEGER, 
			mediaType TEXT, 
			mediaId TEXT, 
			size INTEGER, 
			libraryFolderId TEXT, 
			authorNamesFirstLast TEXT, 
			authorNamesLastFirst TEXT, 
			title TEXT, 
			titleIgnorePrefix TEXT
		)`,
		`CREATE TABLE books (
			id TEXT PRIMARY KEY, 
			title TEXT, 
			titleIgnorePrefix TEXT, 
			subtitle TEXT, 
			publishedYear TEXT, 
			publishedDate TEXT, 
			publisher TEXT, 
			description TEXT, 
			isbn TEXT, 
			asin TEXT, 
			language TEXT, 
			explicit INTEGER, 
			abridged INTEGER, 
			coverPath TEXT, 
			duration REAL, 
			narrators TEXT, 
			audioFiles TEXT, 
			ebookFile TEXT, 
			chapters TEXT, 
			tags TEXT, 
			genres TEXT
		)`,
		`CREATE TABLE podcasts (
			id TEXT PRIMARY KEY, 
			title TEXT, 
			titleIgnorePrefix TEXT, 
			author TEXT, 
			releaseDate TEXT, 
			feedURL TEXT, 
			imageURL TEXT, 
			description TEXT, 
			itunesPageURL TEXT, 
			itunesId TEXT, 
			itunesArtistId TEXT, 
			language TEXT, 
			podcastType TEXT, 
			explicit INTEGER, 
			autoDownloadEpisodes INTEGER, 
			autoDownloadSchedule TEXT, 
			lastEpisodeCheck TEXT, 
			maxEpisodesToKeep INTEGER, 
			maxNewEpisodesToDownload INTEGER, 
			coverPath TEXT, 
			tags TEXT, 
			genres TEXT, 
			numEpisodes INTEGER
		)`,
		`CREATE TABLE podcastEpisodes (
			id TEXT PRIMARY KEY, 
			podcastId TEXT, 
			title TEXT, 
			audioFile TEXT, 
			pubDate TEXT, 
			description TEXT, 
			season TEXT, 
			episode TEXT, 
			episodeType TEXT
		)`,
		`CREATE TABLE playlists (
			id TEXT PRIMARY KEY, 
			name TEXT, 
			description TEXT, 
			createdAt TEXT, 
			updatedAt TEXT, 
			libraryId TEXT, 
			userId TEXT
		)`,
		`CREATE TABLE playlistMediaItems (
			id TEXT PRIMARY KEY, 
			mediaItemId TEXT, 
			mediaItemType TEXT, 
			"order" INTEGER, 
			createdAt TEXT, 
			playlistId TEXT
		)`,
		`CREATE TABLE feeds (
			id TEXT PRIMARY KEY,
			type TEXT,
			entityId TEXT,
			userId TEXT,
			serverAddress TEXT,
			createdAt TEXT,
			updatedAt TEXT
		)`,
		`CREATE TABLE collections (
			id TEXT PRIMARY KEY,
			libraryId TEXT,
			name TEXT,
			description TEXT,
			createdAt TEXT,
			updatedAt TEXT
		)`,
		`CREATE TABLE collectionBooks (
			id TEXT PRIMARY KEY,
			"order" INTEGER,
			createdAt TEXT,
			bookId TEXT,
			collectionId TEXT
		)`,
		`CREATE TABLE series (
			id TEXT PRIMARY KEY,
			libraryId TEXT,
			name TEXT,
			nameIgnorePrefix TEXT,
			description TEXT,
			createdAt TEXT,
			updatedAt TEXT
		)`,
		`CREATE TABLE bookSeries (
			bookId TEXT,
			seriesId TEXT,
			sequence TEXT
		)`,
	}

	for _, schema := range schemas {
		if _, err := db.Exec(schema); err != nil {
			db.Close()
			t.Fatalf("failed to create schema: %v", err)
		}
	}

	return db
}

func insertUser(t *testing.T, db *sql.DB, id, username, uType string, isActive int, permissions string) {
	_, err := db.Exec(`
		INSERT INTO users (id, username, type, isActive, permissions) 
		VALUES (?, ?, ?, ?, ?)
	`, id, username, uType, isActive, permissions)
	if err != nil {
		t.Fatalf("failed to insert user: %v", err)
	}
}

func insertPodcast(t *testing.T, db *sql.DB, id, title, feedURL, description, itunesPageURL, language string) {
	_, err := db.Exec(`
		INSERT INTO podcasts (id, title, feedURL, description, itunesPageURL, language) 
		VALUES (?, ?, ?, ?, ?, ?)
	`, id, title, feedURL, description, itunesPageURL, language)
	if err != nil {
		t.Fatalf("failed to insert podcast: %v", err)
	}
}

func insertPodcastFull(t *testing.T, db *sql.DB, id, title, author, description, language, podcastType string, explicit int, coverPath, feedURL string) {
	_, err := db.Exec(`
		INSERT INTO podcasts (id, title, author, description, language, podcastType, explicit, coverPath, feedURL) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, title, author, description, language, podcastType, explicit, coverPath, feedURL)
	if err != nil {
		t.Fatalf("failed to insert podcast full: %v", err)
	}
}

func insertPodcastEpisode(t *testing.T, db *sql.DB, id, podcastId, title, audioFileJSON, pubDate, description, season, episode, episodeType string) {
	_, err := db.Exec(`
		INSERT INTO podcastEpisodes (id, podcastId, title, audioFile, pubDate, description, season, episode, episodeType)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, podcastId, title, audioFileJSON, pubDate, description, season, episode, episodeType)
	if err != nil {
		t.Fatalf("failed to insert podcast episode: %v", err)
	}
}

func insertLibraryItem(t *testing.T, db *sql.DB, id, libraryId, mediaType, mediaId string) {
	_, err := db.Exec(`
		INSERT INTO libraryItems (id, libraryId, mediaType, mediaId) 
		VALUES (?, ?, ?, ?)
	`, id, libraryId, mediaType, mediaId)
	if err != nil {
		t.Fatalf("failed to insert library item: %v", err)
	}
}

func insertBook(t *testing.T, db *sql.DB, id, title, description, language string, explicit int, coverPath string, duration float64, audioFilesJSON, chaptersJSON string) {
	_, err := db.Exec(`
		INSERT INTO books (id, title, description, language, explicit, coverPath, duration, audioFiles, chapters)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, title, description, language, explicit, coverPath, duration, audioFilesJSON, chaptersJSON)
	if err != nil {
		t.Fatalf("failed to insert book: %v", err)
	}
}

func writeTempFile(t *testing.T, dir, filename string, content []byte) string {
	path := filepath.Join(dir, filename)
	err := os.WriteFile(path, content, 0644)
	if err != nil {
		t.Fatalf("failed to write temp file %s: %v", filename, err)
	}
	return path
}

func makeAudioFileJSON(path, filename, ext, mimeType string, size int64, duration float64) string {
	type MetadataStruct struct {
		Path     string `json:"path"`
		RelPath  string `json:"relPath"`
		Filename string `json:"filename"`
		Ext      string `json:"ext"`
		Size     int64  `json:"size"`
	}
	type AudioFileStruct struct {
		Index    int            `json:"index"`
		Duration float64        `json:"duration"`
		Codec    string         `json:"codec"`
		MimeType string         `json:"mimeType"`
		Metadata MetadataStruct `json:"metadata"`
	}
	af := AudioFileStruct{
		Index:    0,
		Duration: duration,
		Codec:    "mp3",
		MimeType: mimeType,
		Metadata: MetadataStruct{
			Path:     path,
			RelPath:  filename,
			Filename: filename,
			Ext:      ext,
			Size:     size,
		},
	}
	b, _ := json.Marshal(af)
	return string(b)
}

func makeBookTracksJSON(tracks []audiobookTrack) string {
	b, _ := json.Marshal(tracks)
	return string(b)
}

func makeChaptersJSON(chapters []audiobookChapter) string {
	b, _ := json.Marshal(chapters)
	return string(b)
}

func TestGenerateOPML(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	mgr := NewFeedManager(db)
	ctx := context.Background()

	// 1. User not found
	_, err := mgr.GenerateOPML(ctx, "nonexistent", "lib1")
	if err == nil || !strings.Contains(err.Error(), "user not found") {
		t.Errorf("expected user not found error, got: %v", err)
	}

	// 2. Inactive user
	insertUser(t, db, "user_inactive", "inactive", "user", 0, `{"accessAllLibraries": true}`)
	_, err = mgr.GenerateOPML(ctx, "user_inactive", "lib1")
	if err == nil || !strings.Contains(err.Error(), "user is inactive") {
		t.Errorf("expected user is inactive error, got: %v", err)
	}

	// 3. Admin / Root user (should bypass permissions checks)
	insertUser(t, db, "user_admin", "admin", "admin", 1, "")
	// Insert a podcast and library item to verify it can fetch
	insertPodcast(t, db, "pod1", "Podcast Title", "http://example.com/feed.xml", "Podcast Desc", "http://example.com/html", "en")
	insertLibraryItem(t, db, "li1", "lib1", "podcast", "pod1")

	opmlStr, err := mgr.GenerateOPML(ctx, "user_admin", "lib1")
	if err != nil {
		t.Fatalf("unexpected admin OPML generation error: %v", err)
	}
	if !strings.Contains(opmlStr, "Podcast Title") || !strings.Contains(opmlStr, "http://example.com/feed.xml") {
		t.Errorf("expected OPML to contain Podcast Title and feed URL, got: %s", opmlStr)
	}

	// 4. Regular user without permissions JSON
	insertUser(t, db, "user_no_perms", "noperms", "user", 1, "")
	_, err = mgr.GenerateOPML(ctx, "user_no_perms", "lib1")
	if err == nil || !strings.Contains(err.Error(), "user does not have access to library") {
		t.Errorf("expected access denied error, got: %v", err)
	}

	// 5. Regular user with accessAllLibraries true
	insertUser(t, db, "user_all_libs", "alllibs", "user", 1, `{"accessAllLibraries": true}`)
	_, err = mgr.GenerateOPML(ctx, "user_all_libs", "lib1")
	if err != nil {
		t.Errorf("unexpected error for accessAllLibraries: %v", err)
	}

	// 6. Regular user with specific library access
	insertUser(t, db, "user_spec_lib", "speclib", "user", 1, `{"librariesAccessible": ["lib1"]}`)
	_, err = mgr.GenerateOPML(ctx, "user_spec_lib", "lib1")
	if err != nil {
		t.Errorf("unexpected error for specific library access: %v", err)
	}
	_, err = mgr.GenerateOPML(ctx, "user_spec_lib", "lib2")
	if err == nil || !strings.Contains(err.Error(), "user does not have access to library lib2") {
		t.Errorf("expected access denied for lib2, got: %v", err)
	}

	// 7. Verify OPML outline filtering (empty feedURL should be skipped)
	// Insert another podcast with empty feedURL
	insertPodcast(t, db, "pod_empty_feed", "No Feed Podcast", "", "No Feed Desc", "http://example.com/nofeed", "en")
	insertLibraryItem(t, db, "li2", "lib1", "podcast", "pod_empty_feed")

	opmlStr, err = mgr.GenerateOPML(ctx, "user_admin", "lib1")
	if err != nil {
		t.Fatalf("unexpected OPML generation error: %v", err)
	}
	if strings.Contains(opmlStr, "No Feed Podcast") {
		t.Errorf("OPML should not contain podcast without feedURL")
	}
}

func TestServeRSSFeed_Podcast(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tempDir := t.TempDir()

	// 1. Create dummy files
	coverContent := []byte("fake_image_bytes")
	coverPath := writeTempFile(t, tempDir, "cover.jpg", coverContent)

	audioContent := []byte("fake_audio_bytes_abcdefghijklmnopqrstuvwxyz")
	audioPath := writeTempFile(t, tempDir, "track.mp3", audioContent)

	// 2. Set up DB entries
	podcastID := "pod1"
	libraryItemID := "libitem1"

	insertLibraryItem(t, db, libraryItemID, "lib1", "podcast", podcastID)
	// Update createdAt of the libraryItem so it can be parsed as a valid RFC3339 time
	_, err := db.Exec("UPDATE libraryItems SET createdAt = ? WHERE id = ?", "2026-06-09T00:00:00Z", libraryItemID)
	if err != nil {
		t.Fatalf("failed to set libraryItem createdAt: %v", err)
	}

	insertPodcastFull(t, db, podcastID, "The Podcast", "The Author", "A great podcast", "en", "episodic", 1, coverPath, "http://feed.xml")

	audioJSON := makeAudioFileJSON(audioPath, "track.mp3", ".mp3", "audio/mpeg", int64(len(audioContent)), 120.0)
	insertPodcastEpisode(t, db, "ep1", podcastID, "Episode 1", audioJSON, "2026-06-09 12:00:00", "Episode 1 Description", "1", "2", "full")

	mgr := NewFeedManager(db)
	handler := mgr.ServeRSSFeed(libraryItemID)

	// 3. Test XML Feed response
	req := httptest.NewRequest("GET", "/feed/"+libraryItemID, nil)
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK, got: %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/rss+xml; charset=utf-8" {
		t.Errorf("expected Content-Type application/rss+xml; charset=utf-8, got: %q", contentType)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := string(bodyBytes)

	// Validate RSS Headers and iTunes tags
	expectedXMLSnippets := []string{
		`<rss version="2.0"`,
		`xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd"`,
		`xmlns:podcast="https://podcastindex.org/namespace/1.0"`,
		`xmlns:googleplay="http://www.google.com/schemas/play-podcasts/1.0"`,
		`<title>The Podcast</title>`,
		`<description>A great podcast</description>`,
		`<generator>Audiobookshelf</generator>`,
		`<language>en</language>`,
		`<itunes:type>episodic</itunes:type>`,
		`<itunes:author>The Author</itunes:author>`,
		`<itunes:explicit>yes</itunes:explicit>`,
		`<itunes:summary><![CDATA[A great podcast]]></itunes:summary>`,
		`<itunes:image href="http://example.com/feed/` + libraryItemID + `/cover"`,
		`<item>`,
		`<title>Episode 1</title>`,
		`<description>Episode 1 Description</description>`,
		`<itunes:summary><![CDATA[Episode 1 Description]]></itunes:summary>`,
		`<guid>http://example.com/feed/` + libraryItemID + `/item/ep1/media</guid>`,
		`<pubDate>Tue, 09 Jun 2026 12:00:00 +0000</pubDate>`,
		`<enclosure url="http://example.com/feed/` + libraryItemID + `/item/ep1/media.mp3" length="43" type="audio/mpeg"`,
		`<itunes:duration>120</itunes:duration>`,
		`<itunes:season>1</itunes:season>`,
		`<itunes:episode>2</itunes:episode>`,
		`<itunes:episodeType>full</itunes:episodeType>`,
	}

	for _, snippet := range expectedXMLSnippets {
		if !strings.Contains(bodyStr, snippet) {
			t.Errorf("expected feed XML to contain: %s", snippet)
		}
	}

	// 4. Test Cover response
	reqCover := httptest.NewRequest("GET", "/feed/"+libraryItemID+"/cover", nil)
	wCover := httptest.NewRecorder()
	handler(wCover, reqCover)

	respCover := wCover.Result()
	if respCover.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK for cover, got: %d", respCover.StatusCode)
	}
	coverBytes, _ := io.ReadAll(respCover.Body)
	if !bytes.Equal(coverBytes, coverContent) {
		t.Errorf("expected cover content %q, got: %q", string(coverContent), string(coverBytes))
	}

	// Test Cover not found (empty coverPath)
	_, _ = db.Exec("UPDATE podcasts SET coverPath = '' WHERE id = ?", podcastID)
	wCover404 := httptest.NewRecorder()
	handler(wCover404, reqCover)
	if wCover404.Result().StatusCode != http.StatusNotFound {
		t.Errorf("expected cover 404, got: %d", wCover404.Result().StatusCode)
	}
	// Restore cover path
	_, _ = db.Exec("UPDATE podcasts SET coverPath = ? WHERE id = ?", coverPath, podcastID)

	// 5. Test Audio File download
	reqAudio := httptest.NewRequest("GET", "/feed/"+libraryItemID+"/item/ep1/media.mp3", nil)
	wAudio := httptest.NewRecorder()
	handler(wAudio, reqAudio)

	respAudio := wAudio.Result()
	if respAudio.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK for audio, got: %d", respAudio.StatusCode)
	}
	audioMime := respAudio.Header.Get("Content-Type")
	if audioMime != "audio/mpeg" {
		t.Errorf("expected mime audio/mpeg, got: %q", audioMime)
	}
	audioBytes, _ := io.ReadAll(respAudio.Body)
	if !bytes.Equal(audioBytes, audioContent) {
		t.Errorf("expected audio content %q, got: %q", string(audioContent), string(audioBytes))
	}

	// Test range request
	reqAudioRange := httptest.NewRequest("GET", "/feed/"+libraryItemID+"/item/ep1/media.mp3", nil)
	reqAudioRange.Header.Set("Range", "bytes=0-4")
	wAudioRange := httptest.NewRecorder()
	handler(wAudioRange, reqAudioRange)

	respAudioRange := wAudioRange.Result()
	if respAudioRange.StatusCode != http.StatusPartialContent {
		t.Errorf("expected status 206 Partial Content, got: %d", respAudioRange.StatusCode)
	}
	contentRange := respAudioRange.Header.Get("Content-Range")
	if !strings.HasPrefix(contentRange, "bytes 0-4/") {
		t.Errorf("expected Content-Range starting with 'bytes 0-4/', got: %q", contentRange)
	}
	audioRangeBytes, _ := io.ReadAll(respAudioRange.Body)
	expectedRangeBytes := audioContent[0:5]
	if !bytes.Equal(audioRangeBytes, expectedRangeBytes) {
		t.Errorf("expected range content %q, got: %q", string(expectedRangeBytes), string(audioRangeBytes))
	}

	// Test track not found (invalid episodeID)
	reqAudio404 := httptest.NewRequest("GET", "/feed/"+libraryItemID+"/item/invalid_ep/media.mp3", nil)
	wAudio404 := httptest.NewRecorder()
	handler(wAudio404, reqAudio404)
	if wAudio404.Result().StatusCode != http.StatusNotFound {
		t.Errorf("expected track 404, got: %d", wAudio404.Result().StatusCode)
	}
}

func TestServeRSSFeed_Book(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tempDir := t.TempDir()

	coverContent := []byte("book_cover_bytes")
	coverPath := writeTempFile(t, tempDir, "book_cover.jpg", coverContent)

	audioContent := []byte("book_audio_bytes_1234567890")
	audioPath := writeTempFile(t, tempDir, "book_track.mp3", audioContent)

	bookID := "book1"
	libraryItemID := "libitem2"

	insertLibraryItem(t, db, libraryItemID, "lib1", "book", bookID)
	_, _ = db.Exec("UPDATE libraryItems SET authorNamesFirstLast = ?, createdAt = ? WHERE id = ?", "Author Name", "2026-06-09T00:00:00Z", libraryItemID)

	// AudiobookTrack JSON structure
	tracks := []audiobookTrack{
		{
			Index:       0,
			Exclude:     false,
			Duration:    60.0,
			Codec:       "mp3",
			MimeType:    "audio/mpeg",
			StartOffset: 0.0,
			Metadata: struct {
				Path     string `json:"path"`
				Filename string `json:"filename"`
				Size     int64  `json:"size"`
			}{
				Path:     audioPath,
				Filename: "book_track.mp3",
				Size:     int64(len(audioContent)),
			},
		},
		{
			Index:   1,
			Exclude: true, // This track is excluded and should not be in the RSS or queryable
			Metadata: struct {
				Path     string `json:"path"`
				Filename string `json:"filename"`
				Size     int64  `json:"size"`
			}{
				Path:     "excluded.mp3",
				Filename: "excluded.mp3",
				Size:     100,
			},
		},
	}
	audioFilesJSON := makeBookTracksJSON(tracks)

	// Optional chapters
	chapters := []audiobookChapter{
		{
			ID:    0,
			Start: 0.0,
			End:   60.0,
			Title: "Chapter 1 Title",
		},
	}
	chaptersJSON := makeChaptersJSON(chapters)

	insertBook(t, db, bookID, "The Book", "A great book", "en", 0, coverPath, 60.0, audioFilesJSON, chaptersJSON)

	mgr := NewFeedManager(db)
	handler := mgr.ServeRSSFeed(libraryItemID)

	// 1. Test RSS Feed XML
	req := httptest.NewRequest("GET", "/feed/"+libraryItemID, nil)
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got: %d", resp.StatusCode)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := string(bodyBytes)

	// Check book title, description, itunes:author
	if !strings.Contains(bodyStr, "<title>The Book</title>") {
		t.Errorf("expected feed to contain book title")
	}
	if !strings.Contains(bodyStr, "<itunes:author>Author Name</itunes:author>") {
		t.Errorf("expected feed to contain author name")
	}

	// Verify track GUID and enclosure URL
	expectedTrackMD5 := computeMD5(audioPath)
	expectedGUID := "http://example.com/feed/" + libraryItemID + "/item/" + expectedTrackMD5 + "/media"
	if !strings.Contains(bodyStr, expectedGUID) {
		t.Errorf("expected feed to contain track GUID: %s", expectedGUID)
	}

	if !strings.Contains(bodyStr, "<title>book_track</title>") {
		t.Errorf("expected feed to use filename-derived title 'book_track', got: %s", bodyStr)
	}

	// 2. Test Audio File download using computed MD5
	reqAudio := httptest.NewRequest("GET", "/feed/"+libraryItemID+"/item/"+expectedTrackMD5+"/media.mp3", nil)
	wAudio := httptest.NewRecorder()
	handler(wAudio, reqAudio)

	respAudio := wAudio.Result()
	if respAudio.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for book track, got: %d", respAudio.StatusCode)
	}
	audioBytes, _ := io.ReadAll(respAudio.Body)
	if !bytes.Equal(audioBytes, audioContent) {
		t.Errorf("expected track content, got: %q", string(audioBytes))
	}

	// 3. Test Chapter Titles resolution
	tracks3 := []audiobookTrack{
		{
			Index:       0,
			Exclude:     false,
			Duration:    30.0,
			Codec:       "mp3",
			MimeType:    "audio/mpeg",
			StartOffset: 0.0,
			Metadata: struct {
				Path     string `json:"path"`
				Filename string `json:"filename"`
				Size     int64  `json:"size"`
			}{
				Path:     audioPath,
				Filename: "t1.mp3",
				Size:     10,
			},
		},
		{
			Index:       1,
			Exclude:     false,
			Duration:    30.0,
			Codec:       "mp3",
			MimeType:    "audio/mpeg",
			StartOffset: 30.0,
			Metadata: struct {
				Path     string `json:"path"`
				Filename string `json:"filename"`
				Size     int64  `json:"size"`
			}{
				Path:     audioPath,
				Filename: "t2.mp3",
				Size:     10,
			},
		},
	}
	chapters3 := []audiobookChapter{
		{
			ID:    0,
			Start: 0.0,
			End:   30.0,
			Title: "Chapter One Title",
		},
		{
			ID:    1,
			Start: 30.0,
			End:   60.0,
			Title: "Chapter Two Title",
		},
	}
	res, err := db.Exec("UPDATE books SET audioFiles = ?, chapters = ? WHERE id = ?", makeBookTracksJSON(tracks3), makeChaptersJSON(chapters3), bookID)
	if err != nil {
		t.Fatalf("failed to update books: %v", err)
	}
	rowsAffected, _ := res.RowsAffected()
	t.Logf("UPDATE books rows affected: %d", rowsAffected)

	wXML3 := httptest.NewRecorder()
	handler(wXML3, req)
	bodyStr3 := wXML3.Body.String()
	if !strings.Contains(bodyStr3, "<title>Chapter One Title</title>") || !strings.Contains(bodyStr3, "<title>Chapter Two Title</title>") {
		t.Errorf("expected chapter titles to be used for multi-track book, got: %s", bodyStr3)
	}
}

func TestServeRSSFeed_Playlist(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tempDir := t.TempDir()

	// 1. Create dummy files
	coverContent := []byte("podcast_cover_bytes")
	coverPath := writeTempFile(t, tempDir, "podcast_cover.jpg", coverContent)

	audioContentBook := []byte("playlist_book_audio_bytes")
	audioPathBook := writeTempFile(t, tempDir, "play_book_track.mp3", audioContentBook)

	audioContentPodcast := []byte("playlist_podcast_audio_bytes")
	audioPathPodcast := writeTempFile(t, tempDir, "play_podcast_track.mp3", audioContentPodcast)

	// 2. Set up DB
	playlistID := "play1"
	_, err := db.Exec(`
		INSERT INTO playlists (id, name, description, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, ?)
	`, playlistID, "My Playlist", "My description", "2026-06-09T00:00:00Z", "2026-06-09T00:00:00Z")
	if err != nil {
		t.Fatalf("failed to insert playlist: %v", err)
	}

	// First media item: book with NO cover
	bookID := "playbook1"
	_, err = db.Exec(`
		INSERT INTO playlistMediaItems (id, mediaItemId, mediaItemType, "order", createdAt, playlistId)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "pmi1", bookID, "book", 1, "2026-06-09T00:00:00Z", playlistID)
	if err != nil {
		t.Fatalf("failed to insert playlistMediaItem 1: %v", err)
	}

	bookTracks := []audiobookTrack{
		{
			Index:       0,
			Exclude:     false,
			Duration:    45.0,
			Codec:       "mp3",
			MimeType:    "audio/mpeg",
			StartOffset: 0.0,
			Metadata: struct {
				Path     string `json:"path"`
				Filename string `json:"filename"`
				Size     int64  `json:"size"`
			}{
				Path:     audioPathBook,
				Filename: "play_book_track.mp3",
				Size:     int64(len(audioContentBook)),
			},
		},
	}
	insertBook(t, db, bookID, "Playlist Book", "A book in a playlist", "en", 0, "", 45.0, makeBookTracksJSON(bookTracks), "[]")

	// Create a libraryItem for the book since the feed query joins books to libraryItems
	insertLibraryItem(t, db, "libitem_playbook", "lib1", "book", bookID)
	_, _ = db.Exec("UPDATE libraryItems SET createdAt = ? WHERE id = ?", "2026-06-09T00:00:00Z", "libitem_playbook")

	// Second media item: podcast episode
	episodeID := "playep1"
	podcastID := "playpod1"
	_, err = db.Exec(`
		INSERT INTO playlistMediaItems (id, mediaItemId, mediaItemType, "order", createdAt, playlistId)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "pmi2", episodeID, "podcastEpisode", 2, "2026-06-09T00:00:00Z", playlistID)
	if err != nil {
		t.Fatalf("failed to insert playlistMediaItem 2: %v", err)
	}

	insertPodcastFull(t, db, podcastID, "Playlist Podcast", "Playlist Author", "A podcast in playlist", "en", "serial", 0, coverPath, "http://playfeed.xml")
	audioJSON := makeAudioFileJSON(audioPathPodcast, "play_podcast_track.mp3", ".mp3", "audio/mpeg", int64(len(audioContentPodcast)), 90.0)
	insertPodcastEpisode(t, db, episodeID, podcastID, "Playlist Episode 1", audioJSON, "2026-06-09 12:00:00", "Ep in playlist", "1", "1", "full")

	mgr := NewFeedManager(db)
	handler := mgr.ServeRSSFeed(playlistID)

	// 3. Test Playlist Cover (should skip book with empty cover and resolve to podcast cover)
	reqCover := httptest.NewRequest("GET", "/feed/"+playlistID+"/cover", nil)
	wCover := httptest.NewRecorder()
	handler(wCover, reqCover)

	respCover := wCover.Result()
	if respCover.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for playlist cover, got: %d", respCover.StatusCode)
	}
	respCoverBytes, _ := io.ReadAll(respCover.Body)
	if !bytes.Equal(respCoverBytes, coverContent) {
		t.Errorf("expected playlist cover to resolve to podcast cover")
	}

	// 4. Test Playlist RSS XML
	reqXML := httptest.NewRequest("GET", "/feed/"+playlistID, nil)
	wXML := httptest.NewRecorder()
	handler(wXML, reqXML)

	respXML := wXML.Result()
	if respXML.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for playlist XML, got: %d", respXML.StatusCode)
	}
	bodyStr := wXML.Body.String()

	if !strings.Contains(bodyStr, "<title>My Playlist</title>") {
		t.Errorf("expected playlist title in XML")
	}
	if !strings.Contains(bodyStr, "<title>Playlist Episode 1</title>") {
		t.Errorf("expected playlist podcast episode title in XML")
	}
	if !strings.Contains(bodyStr, "<title>Playlist Book</title>") {
		t.Errorf("expected playlist book title in XML")
	}

	// 5. Verify book track GUID inside playlist (uses MD5 with playlistID, bookID, and path)
	expectedBookTrackMD5 := computeMD5(playlistID + "_" + bookID + "_" + audioPathBook)
	expectedBookGUID := "http://example.com/feed/" + playlistID + "/item/" + expectedBookTrackMD5 + "/media"
	if !strings.Contains(bodyStr, expectedBookGUID) {
		t.Errorf("expected playlist RSS XML to contain book track GUID: %s", expectedBookGUID)
	}

	// 6. Test downloading book track inside playlist
	reqBookAudio := httptest.NewRequest("GET", "/feed/"+playlistID+"/item/"+expectedBookTrackMD5+"/media.mp3", nil)
	wBookAudio := httptest.NewRecorder()
	handler(wBookAudio, reqBookAudio)

	if wBookAudio.Result().StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for playlist book track, got: %d", wBookAudio.Result().StatusCode)
	}
	bookAudioBytes, _ := io.ReadAll(wBookAudio.Result().Body)
	if !bytes.Equal(bookAudioBytes, audioContentBook) {
		t.Errorf("expected playlist book track content")
	}

	// 7. Test downloading podcast track inside playlist
	reqPodAudio := httptest.NewRequest("GET", "/feed/"+playlistID+"/item/"+episodeID+"/media.mp3", nil)
	wPodAudio := httptest.NewRecorder()
	handler(wPodAudio, reqPodAudio)

	if wPodAudio.Result().StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for playlist podcast track, got: %d", wPodAudio.Result().StatusCode)
	}
	podAudioBytes, _ := io.ReadAll(wPodAudio.Result().Body)
	if !bytes.Equal(podAudioBytes, audioContentPodcast) {
		t.Errorf("expected playlist podcast track content")
	}
}

func TestServeRSSFeed_AccessControl(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert test data
	libraryItemID := "lib_item_1"
	_, err := db.Exec(`
		INSERT INTO libraryItems (id, title, mediaType, mediaId, createdAt) 
		VALUES (?, 'Access Controlled Book', 'book', 'book_1', '2026-07-10T11:00:00Z')
	`, libraryItemID)
	if err != nil {
		t.Fatalf("failed to insert library item: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO books (id, title, explicit, audioFiles, chapters) 
		VALUES ('book_1', 'Access Controlled Book', 0, '[]', '[]')
	`)
	if err != nil {
		t.Fatalf("failed to insert book: %v", err)
	}

	manager := NewFeedManager(db)

	// 1. Without feed in feeds table, query using a random slug -> should 404
	req404 := httptest.NewRequest("GET", "/feed/non_existent_slug", nil)
	w404 := httptest.NewRecorder()
	manager.ServeRSSFeed("non_existent_slug")(w404, req404)
	if w404.Result().StatusCode != http.StatusNotFound {
		t.Errorf("Expected 404 for non-existent feed, got %d", w404.Result().StatusCode)
	}

	// 2. Query using libraryItemID (which exists in libraryItems table) -> should serve (fallback logic)
	reqFallback := httptest.NewRequest("GET", "/feed/"+libraryItemID, nil)
	wFallback := httptest.NewRecorder()
	manager.ServeRSSFeed(libraryItemID)(wFallback, reqFallback)
	if wFallback.Result().StatusCode != http.StatusOK {
		t.Errorf("Expected 200 via fallback logic, got %d", wFallback.Result().StatusCode)
	}

	// 3. Register a feed with a custom slug
	slug := "my-secret-feed-slug"
	_, err = db.Exec(`
		INSERT INTO feeds (id, type, entityId, userId, serverAddress, createdAt, updatedAt)
		VALUES (?, 'book', ?, 'user_1', 'http://example.com', '2026-07-10T11:00:00Z', '2026-07-10T11:00:00Z')
	`, slug, libraryItemID)
	if err != nil {
		t.Fatalf("failed to insert feed row: %v", err)
	}

	// 4. Query using the custom slug -> should 200 OK
	reqSlug := httptest.NewRequest("GET", "/feed/"+slug, nil)
	wSlug := httptest.NewRecorder()
	manager.ServeRSSFeed(slug)(wSlug, reqSlug)
	if wSlug.Result().StatusCode != http.StatusOK {
		t.Errorf("Expected 200 via custom slug, got %d", wSlug.Result().StatusCode)
	}
}

func TestServeRSSFeed_Collection(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tempDir := t.TempDir()

	coverContent := []byte("collection_cover_bytes")
	coverPath := writeTempFile(t, tempDir, "collection_cover.jpg", coverContent)

	audioContent := []byte("collection_audio_bytes")
	audioPath := writeTempFile(t, tempDir, "coll_track.mp3", audioContent)

	// Set up Collection
	collectionID := "coll1"
	_, err := db.Exec(`
		INSERT INTO collections (id, name, description, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, ?)
	`, collectionID, "My Collection", "My collection description", "2026-06-09T00:00:00Z", "2026-06-09T00:00:00Z")
	if err != nil {
		t.Fatalf("failed to insert collection: %v", err)
	}

	bookID := "collbook1"
	_, err = db.Exec(`
		INSERT INTO collectionBooks (id, collectionId, bookId, "order", createdAt)
		VALUES (?, ?, ?, ?, ?)
	`, "cb1", collectionID, bookID, 1, "2026-06-09T00:00:00Z")
	if err != nil {
		t.Fatalf("failed to insert collectionBook: %v", err)
	}

	bookTracks := []audiobookTrack{
		{
			Index:       0,
			Exclude:     false,
			Duration:    60.0,
			Codec:       "mp3",
			MimeType:    "audio/mpeg",
			StartOffset: 0.0,
			Metadata: struct {
				Path     string `json:"path"`
				Filename string `json:"filename"`
				Size     int64  `json:"size"`
			}{
				Path:     audioPath,
				Filename: "coll_track.mp3",
				Size:     int64(len(audioContent)),
			},
		},
	}
	insertBook(t, db, bookID, "Collection Book", "A book in a collection", "en", 0, coverPath, 60.0, makeBookTracksJSON(bookTracks), "[]")
	insertLibraryItem(t, db, "libitem_collbook", "lib1", "book", bookID)
	_, _ = db.Exec("UPDATE libraryItems SET createdAt = ? WHERE id = ?", "2026-06-09T00:00:00Z", "libitem_collbook")

	mgr := NewFeedManager(db)
	handler := mgr.ServeRSSFeed(collectionID)

	// Test Cover (should resolve to the first book's cover)
	reqCover := httptest.NewRequest("GET", "/feed/"+collectionID+"/cover", nil)
	wCover := httptest.NewRecorder()
	handler(wCover, reqCover)
	if wCover.Result().StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for collection cover, got: %d", wCover.Result().StatusCode)
	}

	// Test RSS XML
	reqXML := httptest.NewRequest("GET", "/feed/"+collectionID, nil)
	wXML := httptest.NewRecorder()
	handler(wXML, reqXML)
	if wXML.Result().StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for collection XML, got: %d", wXML.Result().StatusCode)
	}

	bodyBytes, _ := io.ReadAll(wXML.Result().Body)
	bodyStr := string(bodyBytes)
	if !strings.Contains(bodyStr, "<title>My Collection</title>") {
		t.Errorf("expected collection title in XML")
	}

	// Test download track
	expectedTrackMD5 := computeMD5(collectionID + "_" + bookID + "_" + audioPath)
	reqAudio := httptest.NewRequest("GET", "/feed/"+collectionID+"/item/"+expectedTrackMD5+"/media.mp3", nil)
	wAudio := httptest.NewRecorder()
	handler(wAudio, reqAudio)
	if wAudio.Result().StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for track download, got: %d", wAudio.Result().StatusCode)
	}
}

func TestServeRSSFeed_Series(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tempDir := t.TempDir()

	coverContent := []byte("series_cover_bytes")
	coverPath := writeTempFile(t, tempDir, "series_cover.jpg", coverContent)

	audioContent := []byte("series_audio_bytes")
	audioPath := writeTempFile(t, tempDir, "series_track.mp3", audioContent)

	// Set up Series
	seriesID := "series1"
	_, err := db.Exec(`
		INSERT INTO series (id, name, description, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, ?)
	`, seriesID, "My Series", "My series description", "2026-06-09T00:00:00Z", "2026-06-09T00:00:00Z")
	if err != nil {
		t.Fatalf("failed to insert series: %v", err)
	}

	bookID := "seriesbook1"
	_, err = db.Exec(`
		INSERT INTO bookSeries (bookId, seriesId, sequence)
		VALUES (?, ?, ?)
	`, bookID, seriesID, "1.5")
	if err != nil {
		t.Fatalf("failed to insert bookSeries: %v", err)
	}

	bookTracks := []audiobookTrack{
		{
			Index:       0,
			Exclude:     false,
			Duration:    120.0,
			Codec:       "mp3",
			MimeType:    "audio/mpeg",
			StartOffset: 0.0,
			Metadata: struct {
				Path     string `json:"path"`
				Filename string `json:"filename"`
				Size     int64  `json:"size"`
			}{
				Path:     audioPath,
				Filename: "series_track.mp3",
				Size:     int64(len(audioContent)),
			},
		},
	}
	insertBook(t, db, bookID, "Series Book 1.5", "A book in a series", "en", 0, coverPath, 120.0, makeBookTracksJSON(bookTracks), "[]")
	insertLibraryItem(t, db, "libitem_seriesbook", "lib1", "book", bookID)
	_, _ = db.Exec("UPDATE libraryItems SET createdAt = ? WHERE id = ?", "2026-06-09T00:00:00Z", "libitem_seriesbook")

	mgr := NewFeedManager(db)
	handler := mgr.ServeRSSFeed(seriesID)

	// Test Cover
	reqCover := httptest.NewRequest("GET", "/feed/"+seriesID+"/cover", nil)
	wCover := httptest.NewRecorder()
	handler(wCover, reqCover)
	if wCover.Result().StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for series cover, got: %d", wCover.Result().StatusCode)
	}

	// Test RSS XML
	reqXML := httptest.NewRequest("GET", "/feed/"+seriesID, nil)
	wXML := httptest.NewRecorder()
	handler(wXML, reqXML)
	if wXML.Result().StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for series XML, got: %d", wXML.Result().StatusCode)
	}

	bodyBytes, _ := io.ReadAll(wXML.Result().Body)
	bodyStr := string(bodyBytes)
	if !strings.Contains(bodyStr, "<title>My Series</title>") {
		t.Errorf("expected series title in XML")
	}
	if !strings.Contains(bodyStr, "<title>Book 1.5 - Series Book 1.5</title>") {
		t.Errorf("expected series episode title to contain Book sequence prefix")
	}

	// Test download track
	expectedTrackMD5 := computeMD5(seriesID + "_" + bookID + "_" + audioPath)
	reqAudio := httptest.NewRequest("GET", "/feed/"+seriesID+"/item/"+expectedTrackMD5+"/media.mp3", nil)
	wAudio := httptest.NewRecorder()
	handler(wAudio, reqAudio)
	if wAudio.Result().StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for track download, got: %d", wAudio.Result().StatusCode)
	}
}
