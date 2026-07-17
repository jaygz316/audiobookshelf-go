package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"audiobookshelf/internal/core"
)

func generateTestAudioFile(t *testing.T, dir string, ext string) string {
	path := filepath.Join(dir, "test_audio"+ext)

	var codec string
	if ext == ".mp3" {
		codec = "libmp3lame"
	} else if ext == ".m4a" || ext == ".m4b" || ext == ".mp4" {
		codec = "aac"
	} else {
		codec = "copy"
	}

	// Generate a 0.5s silent audio file
	cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo", "-t", "0.5", "-acodec", codec, path)
	if err := cmd.Run(); err != nil {
		t.Skipf("FFmpeg not available or failed to run: %v", err)
	}
	return path
}

func TestEmbedMetadata(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	testDir := t.TempDir()

	// Seed library folder to allow the test path
	_, err := db.Exec("INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder-1', ?, 'library-1')", testDir)
	if err != nil {
		t.Fatalf("Failed to seed libraryFolders: %v", err)
	}

	audioPath := generateTestAudioFile(t, testDir, ".mp3")

	audioFiles := []map[string]interface{}{
		{
			"ino":      "12345",
			"filename": filepath.Base(audioPath),
			"ext":      ".mp3",
			"size":     1000,
			"metadata": map[string]interface{}{
				"path": audioPath,
			},
		},
	}
	bAudioFiles, _ := json.Marshal(audioFiles)

	chapters := []ChapterInfo{
		{ID: 1, Start: 0.0, End: 0.25, Title: "Introduction"},
		{ID: 2, Start: 0.25, End: 0.5, Title: "Conclusion"},
	}
	bChapters, _ := json.Marshal(chapters)

	genres := []string{"Fiction"}
	bGenres, _ := json.Marshal(genres)

	tags := []string{"Best"}
	bTags, _ := json.Marshal(tags)

	narrators := []string{"Narrator One"}
	bNarrators, _ := json.Marshal(narrators)

	// Create a dummy cover art file inside testDir
	coverPath := filepath.Join(testDir, "cover.jpg")
	coverCmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "color=c=black:s=1x1", "-vframes", "1", coverPath)
	if err := coverCmd.Run(); err != nil {
		t.Skipf("FFmpeg color generation failed: %v", err)
	}

	// Seed database with a book and library item
	_, err = db.Exec(`
		INSERT INTO books (id, title, subtitle, publishedYear, publishedDate, publisher, description, coverPath, narrators, audioFiles, chapters, genres, tags) 
		VALUES ('book-1', 'Test Title', 'Test Subtitle', '2026', '2026-07-10', 'Test Publisher', 'Test Description', ?, ?, ?, ?, ?, ?)`,
		coverPath, bNarrators, bAudioFiles, bChapters, bGenres, bTags)
	if err != nil {
		t.Fatalf("Failed to seed book: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO libraryItems (id, mediaId, mediaType, authorNamesFirstLast, updatedAt) 
		VALUES ('item-1', 'book-1', 'book', 'Author Name', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to seed library item: %v", err)
	}

	userSess := &core.UserSession{
		ID:       "user-1",
		Username: "adminuser",
		Type:     "admin",
		IsActive: true,
	}

	req := httptest.NewRequest("POST", "/api/items/item-1/embed-metadata", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	cfg := &core.Config{
		MetadataPath: t.TempDir(),
	}

	handler := handleEmbedMetadata(db, cfg, "item-1")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp["success"].(bool) != true {
		t.Errorf("Expected success to be true, got %v", resp["success"])
	}

	updated, ok := resp["updatedFiles"].([]interface{})
	if !ok || len(updated) != 1 {
		t.Fatalf("Expected 1 updated file, got %v", resp["updatedFiles"])
	}

	if updated[0].(string) != filepath.Base(audioPath) {
		t.Errorf("Unexpected updated file: %s", updated[0].(string))
	}

	// Verify the file still exists and was not corrupted
	fi, err := os.Stat(audioPath)
	if err != nil {
		t.Fatalf("File does not exist: %v", err)
	}
	if fi.Size() < 500 {
		t.Errorf("File size is too small: %d", fi.Size())
	}
}

func TestEmbedMetadataForbidden(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userSess := &core.UserSession{
		ID:       "user-2",
		Username: "regularuser",
		Type:     "user",
		IsActive: true,
	}

	req := httptest.NewRequest("POST", "/api/items/item-1/embed-metadata", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	cfg := &core.Config{}
	handler := handleEmbedMetadata(db, cfg, "item-1")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected status 403 Forbidden, got %d", rr.Code)
	}
}

func TestEmbedMetadataUnauthorized(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	req := httptest.NewRequest("POST", "/api/items/item-1/embed-metadata", nil)
	rr := httptest.NewRecorder()

	cfg := &core.Config{}
	handler := handleEmbedMetadata(db, cfg, "item-1")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 Unauthorized, got %d", rr.Code)
	}
}

func TestEmbedMetadataRootAccess(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	testDir := t.TempDir()
	_, err := db.Exec("INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder-1', ?, 'library-1')", testDir)
	if err != nil {
		t.Fatalf("Failed to seed libraryFolders: %v", err)
	}

	audioPath := generateTestAudioFile(t, testDir, ".mp3")

	audioFiles := []map[string]interface{}{
		{
			"ino":      "12345",
			"filename": filepath.Base(audioPath),
			"ext":      ".mp3",
			"size":     1000,
			"metadata": map[string]interface{}{
				"path": audioPath,
			},
		},
	}
	bAudioFiles, _ := json.Marshal(audioFiles)

	_, err = db.Exec(`
		INSERT INTO books (id, title, subtitle, publishedYear, publishedDate, publisher, description, coverPath, narrators, audioFiles, chapters, genres, tags) 
		VALUES ('book-1', 'Test Title', '', '', '', '', '', '', '[]', ?, '[]', '[]', '[]')`, bAudioFiles)
	if err != nil {
		t.Fatalf("Failed to seed book: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO libraryItems (id, mediaId, mediaType, authorNamesFirstLast, updatedAt) 
		VALUES ('item-1', 'book-1', 'book', 'Author Name', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to seed library item: %v", err)
	}

	userSess := &core.UserSession{
		ID:       "user-1",
		Username: "rootuser",
		Type:     "root",
		IsActive: true,
	}

	req := httptest.NewRequest("POST", "/api/items/item-1/embed-metadata", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	cfg := &core.Config{
		MetadataPath: t.TempDir(),
	}

	handler := handleEmbedMetadata(db, cfg, "item-1")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK for root user, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestEmbedMetadataItemNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userSess := &core.UserSession{
		ID:       "user-1",
		Username: "adminuser",
		Type:     "admin",
		IsActive: true,
	}

	req := httptest.NewRequest("POST", "/api/items/non-existent-item/embed-metadata", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	cfg := &core.Config{}
	handler := handleEmbedMetadata(db, cfg, "non-existent-item")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 Not Found, got %d", rr.Code)
	}
}

func TestEmbedMetadataNotABook(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.Exec(`
		INSERT INTO libraryItems (id, mediaId, mediaType, authorNamesFirstLast, updatedAt) 
		VALUES ('item-1', 'podcast-1', 'podcast', 'Author Name', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to seed library item: %v", err)
	}

	userSess := &core.UserSession{
		ID:       "user-1",
		Username: "adminuser",
		Type:     "admin",
		IsActive: true,
	}

	req := httptest.NewRequest("POST", "/api/items/item-1/embed-metadata", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	cfg := &core.Config{}
	handler := handleEmbedMetadata(db, cfg, "item-1")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 Bad Request, got %d", rr.Code)
	}
}

func TestEmbedMetadataNoAudioFiles(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.Exec(`
		INSERT INTO books (id, title, subtitle, publishedYear, publishedDate, publisher, description, coverPath, narrators, audioFiles, chapters, genres, tags) 
		VALUES ('book-1', 'Test Title', '', '', '', '', '', '', '[]', '[]', '[]', '[]', '[]')`)
	if err != nil {
		t.Fatalf("Failed to seed book: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO libraryItems (id, mediaId, mediaType, authorNamesFirstLast, updatedAt) 
		VALUES ('item-1', 'book-1', 'book', 'Author Name', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to seed library item: %v", err)
	}

	userSess := &core.UserSession{
		ID:       "user-1",
		Username: "adminuser",
		Type:     "admin",
		IsActive: true,
	}

	req := httptest.NewRequest("POST", "/api/items/item-1/embed-metadata", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	cfg := &core.Config{}
	handler := handleEmbedMetadata(db, cfg, "item-1")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 Bad Request, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestEmbedMetadataUnsafeCoverPath(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	testDir := t.TempDir()
	_, err := db.Exec("INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder-1', ?, 'library-1')", testDir)
	if err != nil {
		t.Fatalf("Failed to seed libraryFolders: %v", err)
	}

	audioPath := generateTestAudioFile(t, testDir, ".mp3")

	audioFiles := []map[string]interface{}{
		{
			"ino":      "12345",
			"filename": filepath.Base(audioPath),
			"ext":      ".mp3",
			"size":     1000,
			"metadata": map[string]interface{}{
				"path": audioPath,
			},
		},
	}
	bAudioFiles, _ := json.Marshal(audioFiles)

	_, err = db.Exec(`
		INSERT INTO books (id, title, subtitle, publishedYear, publishedDate, publisher, description, coverPath, narrators, audioFiles, chapters, genres, tags) 
		VALUES ('book-1', 'Test Title', '', '', '', '', '', '/etc/passwd', '[]', ?, '[]', '[]', '[]')`, bAudioFiles)
	if err != nil {
		t.Fatalf("Failed to seed book: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO libraryItems (id, mediaId, mediaType, authorNamesFirstLast, updatedAt) 
		VALUES ('item-1', 'book-1', 'book', 'Author Name', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to seed library item: %v", err)
	}

	userSess := &core.UserSession{
		ID:       "user-1",
		Username: "adminuser",
		Type:     "admin",
		IsActive: true,
	}

	req := httptest.NewRequest("POST", "/api/items/item-1/embed-metadata", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	cfg := &core.Config{
		MetadataPath: t.TempDir(),
	}

	handler := handleEmbedMetadata(db, cfg, "item-1")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestEmbedMetadataUnsafeAudioFilePath(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	testDir := t.TempDir()
	_, err := db.Exec("INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder-1', ?, 'library-1')", testDir)
	if err != nil {
		t.Fatalf("Failed to seed libraryFolders: %v", err)
	}

	audioPath := generateTestAudioFile(t, testDir, ".mp3")
	unsafeAudioPath := "/etc/passwd"

	audioFiles := []map[string]interface{}{
		{
			"ino":      "12345",
			"filename": filepath.Base(audioPath),
			"ext":      ".mp3",
			"size":     1000,
			"metadata": map[string]interface{}{
				"path": audioPath,
			},
		},
		{
			"ino":      "67890",
			"filename": "passwd",
			"ext":      "",
			"size":     1000,
			"metadata": map[string]interface{}{
				"path": unsafeAudioPath,
			},
		},
	}
	bAudioFiles, _ := json.Marshal(audioFiles)

	_, err = db.Exec(`
		INSERT INTO books (id, title, subtitle, publishedYear, publishedDate, publisher, description, coverPath, narrators, audioFiles, chapters, genres, tags) 
		VALUES ('book-1', 'Test Title', '', '', '', '', '', '', '[]', ?, '[]', '[]', '[]')`, bAudioFiles)
	if err != nil {
		t.Fatalf("Failed to seed book: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO libraryItems (id, mediaId, mediaType, authorNamesFirstLast, updatedAt) 
		VALUES ('item-1', 'book-1', 'book', 'Author Name', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to seed library item: %v", err)
	}

	userSess := &core.UserSession{
		ID:       "user-1",
		Username: "adminuser",
		Type:     "admin",
		IsActive: true,
	}

	req := httptest.NewRequest("POST", "/api/items/item-1/embed-metadata", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	cfg := &core.Config{
		MetadataPath: t.TempDir(),
	}

	handler := handleEmbedMetadata(db, cfg, "item-1")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["success"].(bool) != true {
		t.Errorf("Expected success true, got %v", resp["success"])
	}

	updated, ok := resp["updatedFiles"].([]interface{})
	if !ok {
		t.Fatalf("Expected updatedFiles to be array")
	}

	if len(updated) != 1 {
		t.Errorf("Expected exactly 1 updated file, got %d (%v)", len(updated), updated)
	} else if updated[0].(string) != filepath.Base(audioPath) {
		t.Errorf("Expected %s, got %s", filepath.Base(audioPath), updated[0])
	}
}

func TestEmbedMetadataFFmpegFailureCleanup(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	testDir := t.TempDir()
	_, err := db.Exec("INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder-1', ?, 'library-1')", testDir)
	if err != nil {
		t.Fatalf("Failed to seed libraryFolders: %v", err)
	}

	audioPath := filepath.Join(testDir, "bad_audio.mp3")
	err = os.WriteFile(audioPath, []byte("this is not a valid mp3 file or audio content at all"), 0644)
	if err != nil {
		t.Fatalf("Failed to write mock file: %v", err)
	}

	audioFiles := []map[string]interface{}{
		{
			"ino":      "12345",
			"filename": filepath.Base(audioPath),
			"ext":      ".mp3",
			"size":     1000,
			"metadata": map[string]interface{}{
				"path": audioPath,
			},
		},
	}
	bAudioFiles, _ := json.Marshal(audioFiles)

	_, err = db.Exec(`
		INSERT INTO books (id, title, subtitle, publishedYear, publishedDate, publisher, description, coverPath, narrators, audioFiles, chapters, genres, tags) 
		VALUES ('book-1', 'Test Title', '', '', '', '', '', '', '[]', ?, '[]', '[]', '[]')`, bAudioFiles)
	if err != nil {
		t.Fatalf("Failed to seed book: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO libraryItems (id, mediaId, mediaType, authorNamesFirstLast, updatedAt) 
		VALUES ('item-1', 'book-1', 'book', 'Author Name', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to seed library item: %v", err)
	}

	userSess := &core.UserSession{
		ID:       "user-1",
		Username: "adminuser",
		Type:     "admin",
		IsActive: true,
	}

	req := httptest.NewRequest("POST", "/api/items/item-1/embed-metadata", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	cfg := &core.Config{
		MetadataPath: t.TempDir(),
	}

	handler := handleEmbedMetadata(db, cfg, "item-1")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("Expected status 500 Internal Server Error, got %d: %s", rr.Code, rr.Body.String())
	}

	embedPath := audioPath + ".embed.mp3"
	if _, err := os.Stat(embedPath); !os.IsNotExist(err) {
		t.Errorf("Expected temporary embed file %s to be deleted, but it still exists", embedPath)
	}
}

func TestEmbedMetadataTempFileCleanup(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	testDir := t.TempDir()
	_, err := db.Exec("INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder-1', ?, 'library-1')", testDir)
	if err != nil {
		t.Fatalf("Failed to seed libraryFolders: %v", err)
	}

	audioPath := generateTestAudioFile(t, testDir, ".mp3")
	audioFiles := []map[string]interface{}{
		{
			"ino":      "12345",
			"filename": filepath.Base(audioPath),
			"ext":      ".mp3",
			"size":     1000,
			"metadata": map[string]interface{}{
				"path": audioPath,
			},
		},
	}
	bAudioFiles, _ := json.Marshal(audioFiles)

	_, err = db.Exec(`
		INSERT INTO books (id, title, subtitle, publishedYear, publishedDate, publisher, description, coverPath, narrators, audioFiles, chapters, genres, tags) 
		VALUES ('book-1', 'Test Title', '', '', '', '', '', '', '[]', ?, '[]', '[]', '[]')`, bAudioFiles)
	if err != nil {
		t.Fatalf("Failed to seed book: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO libraryItems (id, mediaId, mediaType, authorNamesFirstLast, updatedAt) 
		VALUES ('item-1', 'book-1', 'book', 'Author Name', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to seed library item: %v", err)
	}

	userSess := &core.UserSession{
		ID:       "user-1",
		Username: "adminuser",
		Type:     "admin",
		IsActive: true,
	}

	getTempFiles := func() map[string]bool {
		files := make(map[string]bool)
		matches, _ := filepath.Glob(filepath.Join(os.TempDir(), "ffmetadata-*.txt"))
		for _, m := range matches {
			files[m] = true
		}
		return files
	}

	beforeFiles := getTempFiles()

	req := httptest.NewRequest("POST", "/api/items/item-1/embed-metadata", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	cfg := &core.Config{
		MetadataPath: t.TempDir(),
	}

	handler := handleEmbedMetadata(db, cfg, "item-1")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	afterFiles := getTempFiles()

	for file := range afterFiles {
		if !beforeFiles[file] {
			t.Errorf("Temporary file was left behind: %s", file)
			os.Remove(file)
		}
	}
}

func TestEscapeMetadataValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "Hello World"},
		{"Backslash \\", "Backslash \\\\"},
		{"Equals =", "Equals \\="},
		{"Semicolon ;", "Semicolon \\;"},
		{"Hash #", "Hash \\#"},
		{"Newline \n", "Newline \\\n"},
		{"Mix \\ = ; # \n", "Mix \\\\ \\= \\; \\# \\\n"},
	}

	for _, tc := range tests {
		got := escapeMetadataValue(tc.input)
		if got != tc.expected {
			t.Errorf("escapeMetadataValue(%q) = %q; expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestWriteFFMetadataFile_Escaping(t *testing.T) {
	meta := &bookEmbedMetadata{
		Title:      "Title with = and ; and # and \\ and \n newline",
		AuthorName: "Artist with \\ and \n newline",
		Narrators:  []string{"Narrator 1"},
		Chapters: []ChapterInfo{
			{ID: 1, Start: 0.0, End: 0.5, Title: "Chapter with ="},
		},
	}

	path, err := writeFFMetadataFile(meta)
	if err != nil {
		t.Fatalf("writeFFMetadataFile failed: %v", err)
	}
	defer os.Remove(path)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read generated FFMETADATA file: %v", err)
	}

	contentStr := string(content)

	expectedTitleLine := "title=Title with \\= and \\; and \\# and \\\\ and \\\n newline"
	expectedArtistLine := "artist=Artist with \\\\ and \\\n newline"
	expectedChapterTitleLine := "title=Chapter with \\="

	if !strings.Contains(contentStr, expectedTitleLine) {
		t.Errorf("FFMETADATA file does not contain expected escaped title line.\nExpected line: %q\nFile contents:\n%s", expectedTitleLine, contentStr)
	}

	if !strings.Contains(contentStr, expectedArtistLine) {
		t.Errorf("FFMETADATA file does not contain expected escaped artist line.\nExpected line: %q\nFile contents:\n%s", expectedArtistLine, contentStr)
	}

	if !strings.Contains(contentStr, expectedChapterTitleLine) {
		t.Errorf("FFMETADATA file does not contain expected escaped chapter title line.\nExpected line: %q\nFile contents:\n%s", expectedChapterTitleLine, contentStr)
	}
}

func findTagValue(probeResult map[string]interface{}, key string) string {
	if format, ok := probeResult["format"].(map[string]interface{}); ok {
		if tags, ok := format["tags"].(map[string]interface{}); ok {
			for k, v := range tags {
				if strings.EqualFold(k, key) {
					if s, ok := v.(string); ok {
						return s
					}
				}
			}
		}
	}
	if streams, ok := probeResult["streams"].([]interface{}); ok {
		for _, s := range streams {
			if stream, ok := s.(map[string]interface{}); ok {
				if tags, ok := stream["tags"].(map[string]interface{}); ok {
					for k, v := range tags {
						if strings.EqualFold(k, key) {
							if val, ok := v.(string); ok {
								return val
							}
						}
					}
				}
			}
		}
	}
	return ""
}

func TestEmbedMetadata_Formats(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	formats := []string{".mp3", ".m4b", ".m4a", ".mp4"}
	for _, ext := range formats {
		t.Run(ext, func(t *testing.T) {
			testDir := t.TempDir()

			_, err := db.Exec("INSERT INTO libraryFolders (id, path, libraryId) VALUES (?, ?, 'library-1')", "folder-"+ext, testDir)
			if err != nil {
				t.Fatalf("Failed to seed libraryFolders: %v", err)
			}
			defer db.Exec("DELETE FROM libraryFolders WHERE id = ?", "folder-"+ext)

			audioPath := generateTestAudioFile(t, testDir, ext)

			meta := &bookEmbedMetadata{
				Title:      "Test Title with = ; # \\ \n Special Characters",
				AuthorName: "Test Artist with = ; # \\ \n",
				Narrators:  []string{"Narrator = 1"},
				AudioFiles: []map[string]interface{}{
					{
						"metadata": map[string]interface{}{
							"path": audioPath,
						},
					},
				},
				Chapters: []ChapterInfo{
					{ID: 1, Start: 0.0, End: 0.2, Title: "Ch = 1"},
					{ID: 2, Start: 0.2, End: 0.5, Title: "Ch = 2"},
				},
				Genres: []string{"Fiction"},
				Tags:   []string{"Tag"},
			}

			metaFilePath, err := writeFFMetadataFile(meta)
			if err != nil {
				t.Fatalf("Failed to write FFMETADATA file: %v", err)
			}
			defer os.Remove(metaFilePath)

			coverPath := filepath.Join(testDir, "cover.jpg")
			coverCmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "color=c=black:s=10x10", "-vframes", "1", coverPath)
			if err := coverCmd.Run(); err != nil {
				t.Skipf("FFmpeg color generation failed: %v", err)
			}

			cfg := &core.Config{
				MetadataPath: t.TempDir(),
			}

			updatedName, err := embedMetadataInAudioFile(db, cfg, meta.AudioFiles[0], metaFilePath, coverPath, true)
			if err != nil {
				t.Fatalf("embedMetadataInAudioFile failed: %v", err)
			}

			if updatedName != filepath.Base(audioPath) {
				t.Errorf("Expected updated filename %q, got %q", filepath.Base(audioPath), updatedName)
			}

			fi, err := os.Stat(audioPath)
			if err != nil {
				t.Fatalf("File does not exist: %v", err)
			}
			if fi.Size() < 1024 {
				t.Errorf("Output file size is too small: %d bytes", fi.Size())
			}

			probeCmd := exec.Command("ffprobe", "-show_format", "-show_chapters", "-show_streams", "-print_format", "json", audioPath)
			output, err := probeCmd.Output()
			if err != nil {
				t.Fatalf("ffprobe failed on output file: %v, output: %s", err, string(output))
			}

			var probeResult map[string]interface{}
			if err := json.Unmarshal(output, &probeResult); err != nil {
				t.Fatalf("Failed to parse ffprobe JSON output: %v", err)
			}

			titleVal := findTagValue(probeResult, "title")
			if titleVal == "" {
				t.Errorf("Title tag not found in ffprobe output")
			} else if !strings.Contains(titleVal, "Test Title") {
				t.Errorf("Expected title to contain 'Test Title', got %q", titleVal)
			}

			artistVal := findTagValue(probeResult, "artist")
			if artistVal == "" {
				t.Errorf("Artist tag not found in ffprobe output")
			} else if !strings.Contains(artistVal, "Test Artist") {
				t.Errorf("Expected artist to contain 'Test Artist', got %q", artistVal)
			}

			chaptersRaw, ok := probeResult["chapters"].([]interface{})
			if !ok || len(chaptersRaw) != 2 {
				t.Errorf("Expected 2 chapters, got %d", len(chaptersRaw))
			} else {
				ch1, ok := chaptersRaw[0].(map[string]interface{})
				if !ok {
					t.Errorf("Expected chapter 0 to be a map")
				} else {
					tags, _ := ch1["tags"].(map[string]interface{})
					chTitle := ""
					for k, v := range tags {
						if strings.EqualFold(k, "title") {
							chTitle, _ = v.(string)
						}
					}
					if !strings.Contains(chTitle, "Ch = 1") {
						t.Errorf("Expected chapter 1 title to contain 'Ch = 1', got %q", chTitle)
					}
				}
			}

			streamsRaw, ok := probeResult["streams"].([]interface{})
			hasCoverStream := false
			if ok {
				for _, s := range streamsRaw {
					if stream, ok := s.(map[string]interface{}); ok {
						if codecType, ok := stream["codec_type"].(string); ok && codecType == "video" {
							hasCoverStream = true
							break
						}
					}
				}
			}
			if !hasCoverStream {
				t.Errorf("Expected video stream for cover art, but none found")
			}
		})
	}
}

func TestEmbedMetadata_Truncation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	testDir := t.TempDir()
	_, err := db.Exec("INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder-trunc', ?, 'library-1')", testDir)
	if err != nil {
		t.Fatalf("Failed to seed libraryFolders: %v", err)
	}

	audioPath := generateTestAudioFile(t, testDir, ".mp3")
	originalStat, err := os.Stat(audioPath)
	if err != nil {
		t.Fatalf("Failed to stat original file: %v", err)
	}

	af := map[string]interface{}{
		"metadata": map[string]interface{}{
			"path": audioPath,
		},
	}

	cfg := &core.Config{
		MetadataPath: t.TempDir(),
	}

	_, err = embedMetadataInAudioFile(db, cfg, af, "/non/existent/path.txt", "", false)
	if err == nil {
		t.Fatal("Expected embedMetadataInAudioFile to fail when metaFilePath is invalid, but it succeeded")
	}

	newStat, err := os.Stat(audioPath)
	if err != nil {
		t.Fatalf("Original file was deleted: %v", err)
	}
	if newStat.Size() != originalStat.Size() {
		t.Errorf("Original file size changed from %d to %d", originalStat.Size(), newStat.Size())
	}

	tmpPath := audioPath + ".embed.mp3"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("Temporary file %s was not cleaned up", tmpPath)
	}
}

func TestCopyFile(t *testing.T) {
	tempDir := t.TempDir()
	src := filepath.Join(tempDir, "src.txt")
	dst := filepath.Join(tempDir, "dst.txt")

	content := []byte("Hello, this is a test of the copyFile function.")
	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatalf("Failed to write src file: %v", err)
	}

	err := copyFile(src, dst)
	if err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	dstContent, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("Failed to read dst file: %v", err)
	}

	if string(dstContent) != string(content) {
		t.Errorf("Expected copy content %q, got %q", string(content), string(dstContent))
	}

	err = copyFile(filepath.Join(tempDir, "nonexistent.txt"), dst)
	if err == nil {
		t.Error("Expected copyFile to fail with non-existent source, but it succeeded")
	}

	err = copyFile(src, filepath.Join(tempDir, "nonexistent_dir", "dst.txt"))
	if err == nil {
		t.Error("Expected copyFile to fail with invalid destination path, but it succeeded")
	}
}
