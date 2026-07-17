package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"audiobookshelf/internal/core"
)

func TestCheckPathExists_BasicAndEdgeCases(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tempDir := t.TempDir()
	libraryFolder := filepath.Join(tempDir, "my-library")
	err := os.MkdirAll(libraryFolder, 0755)
	if err != nil {
		t.Fatalf("Failed to create test library folder: %v", err)
	}

	// Insert library folder and setup permissions
	_, err = db.Exec(`INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder-1', ?, 'lib-1')`, libraryFolder)
	if err != nil {
		t.Fatalf("Failed to insert library folder: %v", err)
	}

	userSess := &core.UserSession{
		ID:                 "user-1",
		Username:           "test-user",
		Type:               "user",
		IsActive:           true,
		AccessAllLibraries: true,
	}

	// 1. Unauthorized access (no context session)
	{
		reqBody := `{"directory": "book1", "folderPath": "` + filepath.ToSlash(libraryFolder) + `"}`
		req := httptest.NewRequest("POST", "/api/filesystem/pathexists", bytes.NewBufferString(reqBody))
		rr := httptest.NewRecorder()

		handleCheckPathExists(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401 StatusUnauthorized, got %d", rr.Code)
		}
	}

	// 2. Empty directory or folderPath
	{
		reqBody := `{"directory": "", "folderPath": "` + filepath.ToSlash(libraryFolder) + `"}`
		req := httptest.NewRequest("POST", "/api/filesystem/pathexists", bytes.NewBufferString(reqBody))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handleCheckPathExists(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected 400 StatusBadRequest, got %d", rr.Code)
		}
	}

	// 3. Folder path not in DB
	{
		nonExistentFolder := filepath.Join(tempDir, "not-in-db")
		reqBody := `{"directory": "book1", "folderPath": "` + filepath.ToSlash(nonExistentFolder) + `"}`
		req := httptest.NewRequest("POST", "/api/filesystem/pathexists", bytes.NewBufferString(reqBody))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handleCheckPathExists(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected 404 StatusNotFound, got %d", rr.Code)
		}
	}

	// 4. Forbidden access (user has no library access)
	{
		restrictedUser := &core.UserSession{
			ID:                  "user-2",
			Username:            "restricted",
			Type:                "user",
			IsActive:            true,
			AccessAllLibraries:  false,
			LibrariesAccessible: []string{"lib-other"},
		}
		reqBody := `{"directory": "book1", "folderPath": "` + filepath.ToSlash(libraryFolder) + `"}`
		req := httptest.NewRequest("POST", "/api/filesystem/pathexists", bytes.NewBufferString(reqBody))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, restrictedUser))
		rr := httptest.NewRecorder()

		handleCheckPathExists(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 StatusForbidden, got %d", rr.Code)
		}
	}

	// 5. Existing file checks
	{
		// Create physical directory inside library folder
		bookDir := filepath.Join(libraryFolder, "Book One")
		_ = os.MkdirAll(bookDir, 0755)

		reqBody := `{"directory": "Book One", "folderPath": "` + filepath.ToSlash(libraryFolder) + `"}`
		req := httptest.NewRequest("POST", "/api/filesystem/pathexists", bytes.NewBufferString(reqBody))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handleCheckPathExists(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", rr.Code)
		}

		var res map[string]interface{}
		json.Unmarshal(rr.Body.Bytes(), &res)
		if res["exists"] != true {
			t.Errorf("Expected exists to be true, got %v", res["exists"])
		}
	}

	// 6. Non-existing path checks
	{
		reqBody := `{"directory": "NonExistentBook", "folderPath": "` + filepath.ToSlash(libraryFolder) + `"}`
		req := httptest.NewRequest("POST", "/api/filesystem/pathexists", bytes.NewBufferString(reqBody))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handleCheckPathExists(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", rr.Code)
		}

		var res map[string]interface{}
		json.Unmarshal(rr.Body.Bytes(), &res)
		if res["exists"] != false {
			t.Errorf("Expected exists to be false, got %v", res["exists"])
		}
	}

	// 7. Path Traversal attempts
	{
		reqBody := `{"directory": "../outside", "folderPath": "` + filepath.ToSlash(libraryFolder) + `"}`
		req := httptest.NewRequest("POST", "/api/filesystem/pathexists", bytes.NewBufferString(reqBody))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handleCheckPathExists(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected 400 StatusBadRequest for path traversal, got %d", rr.Code)
		}
	}

	// 8. DB Subpath Match
	{
		// Seed a libraryItem in the database
		subpath := filepath.ToSlash(filepath.Join(libraryFolder, "AuthorName", "BookTitle"))
		_, err = db.Exec(`
			INSERT INTO libraryItems (id, libraryId, path, title)
			VALUES ('item-1', 'lib-1', ?, 'Database Book Match')
		`, subpath)
		if err != nil {
			t.Fatalf("Failed to seed library item: %v", err)
		}

		// Query for "AuthorName/BookTitle/ExtraFile" which does not physically exist
		reqBody := `{"directory": "AuthorName/BookTitle/ExtraFile", "folderPath": "` + filepath.ToSlash(libraryFolder) + `"}`
		req := httptest.NewRequest("POST", "/api/filesystem/pathexists", bytes.NewBufferString(reqBody))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handleCheckPathExists(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", rr.Code)
		}

		var res map[string]interface{}
		json.Unmarshal(rr.Body.Bytes(), &res)
		if res["exists"] != true {
			t.Errorf("Expected exists to be true via DB match, got %v", res["exists"])
		}
		if res["libraryItemTitle"] != "Database Book Match" {
			t.Errorf("Expected libraryItemTitle to be 'Database Book Match', got %v", res["libraryItemTitle"])
		}
	}
}

func TestCheckPathExists_SymlinkInformationLeak(t *testing.T) {
	// Adversarial test: Check if a symlink pointing outside the library folder leaks file existence.
	db := setupTestDB(t)
	defer db.Close()

	tempDir := t.TempDir()
	libraryFolder := filepath.Join(tempDir, "library")
	privateFolder := filepath.Join(tempDir, "private")

	err := os.MkdirAll(libraryFolder, 0755)
	if err != nil {
		t.Fatalf("Failed to create library folder: %v", err)
	}
	err = os.MkdirAll(privateFolder, 0755)
	if err != nil {
		t.Fatalf("Failed to create private folder: %v", err)
	}

	secretFile := filepath.Join(privateFolder, "secret.txt")
	err = os.WriteFile(secretFile, []byte("super secret contents"), 0644)
	if err != nil {
		t.Fatalf("Failed to create secret file: %v", err)
	}

	// Create symlink inside libraryFolder pointing to privateFolder/secret.txt
	symlinkPath := filepath.Join(libraryFolder, "secret_link")
	err = os.Symlink(secretFile, symlinkPath)
	if err != nil {
		t.Skip("Symlink creation not supported in this environment: ", err)
		return
	}

	// Insert library folder and setup permissions
	_, err = db.Exec(`INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder-1', ?, 'lib-1')`, libraryFolder)
	if err != nil {
		t.Fatalf("Failed to insert library folder: %v", err)
	}

	userSess := &core.UserSession{
		ID:                 "user-1",
		Username:           "test-user",
		Type:               "user",
		IsActive:           true,
		AccessAllLibraries: true,
	}

	reqBody := `{"directory": "secret_link", "folderPath": "` + filepath.ToSlash(libraryFolder) + `"}`
	req := httptest.NewRequest("POST", "/api/filesystem/pathexists", bytes.NewBufferString(reqBody))
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	handleCheckPathExists(db).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", rr.Code)
	}

	var res map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &res)

	// Note: We expect/observe this to be true because of os.Stat following symlinks.
	// We report this as a critical vulnerability/leak potential in the final report!
	t.Logf("Symlink exists result: %v", res["exists"])
}

func TestGetFilesystem_BasicAndEdgeCases(t *testing.T) {
	appRoot := t.TempDir()
	metaDir := filepath.Join(appRoot, "metadata")
	_ = os.MkdirAll(metaDir, 0755)

	safeDir := filepath.Join(appRoot, "safe-directory")
	_ = os.MkdirAll(safeDir, 0755)

	nestedSafeDir := filepath.Join(safeDir, "nested")
	_ = os.MkdirAll(nestedSafeDir, 0755)

	adminSess := &core.UserSession{
		ID:       "admin-1",
		Username: "admin",
		Type:     "admin",
		IsActive: true,
	}

	normalUserSess := &core.UserSession{
		ID:       "user-1",
		Username: "user",
		Type:     "user",
		IsActive: true,
	}

	// 1. Forbidden access for non-admin
	{
		req := httptest.NewRequest("GET", "/api/filesystem?path="+filepath.ToSlash(safeDir), nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, normalUserSess))
		rr := httptest.NewRecorder()

		handleGetFilesystem(appRoot).ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 StatusForbidden, got %d", rr.Code)
		}
	}

	// 2. Relative path should return Bad Request
	{
		req := httptest.NewRequest("GET", "/api/filesystem?path=relative/path", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSess))
		rr := httptest.NewRecorder()

		handleGetFilesystem(appRoot).ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected 400 StatusBadRequest, got %d", rr.Code)
		}
	}

	// 3. Excluded directory under appRoot
	{
		req := httptest.NewRequest("GET", "/api/filesystem?path="+filepath.ToSlash(metaDir), nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSess))
		rr := httptest.NewRecorder()

		handleGetFilesystem(appRoot).ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 StatusForbidden, got %d", rr.Code)
		}
	}

	// 4. Nested under excluded directory
	{
		nestedMetaDir := filepath.Join(metaDir, "nested")
		_ = os.MkdirAll(nestedMetaDir, 0755)

		req := httptest.NewRequest("GET", "/api/filesystem?path="+filepath.ToSlash(nestedMetaDir), nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSess))
		rr := httptest.NewRecorder()

		handleGetFilesystem(appRoot).ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 StatusForbidden for nested excluded directory, got %d", rr.Code)
		}
	}

	// 5. Safe directory access
	{
		req := httptest.NewRequest("GET", "/api/filesystem?path="+filepath.ToSlash(safeDir), nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSess))
		rr := httptest.NewRecorder()

		handleGetFilesystem(appRoot).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", rr.Code)
		}

		var response struct {
			Posix       bool            `json:"posix"`
			Directories []DirectoryInfo `json:"directories"`
		}
		err := json.Unmarshal(rr.Body.Bytes(), &response)
		if err != nil {
			t.Fatalf("Failed to parse response body: %v", err)
		}

		if len(response.Directories) != 1 {
			t.Errorf("Expected 1 directory, got %d", len(response.Directories))
		} else {
			if response.Directories[0].Dirname != "nested" {
				t.Errorf("Expected directory name 'nested', got %q", response.Directories[0].Dirname)
			}
		}
	}
}

func TestGetFilesystem_SymlinkBypassExclusion(t *testing.T) {
	appRoot := t.TempDir()

	// Create excluded directory metadata
	metaDir := filepath.Join(appRoot, "metadata")
	err := os.MkdirAll(metaDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create metadata dir: %v", err)
	}

	// Create a dummy subdirectory inside metadata to check if we can list it
	secretSubDir := filepath.Join(metaDir, "secret-sub")
	err = os.MkdirAll(secretSubDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create secret sub dir: %v", err)
	}

	// Create a safe directory
	safeDir := filepath.Join(appRoot, "safe-directory")
	err = os.MkdirAll(safeDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create safe dir: %v", err)
	}

	// Create a symlink in safeDir pointing to metadata
	symlinkPath := filepath.Join(safeDir, "link_to_metadata")
	err = os.Symlink(metaDir, symlinkPath)
	if err != nil {
		t.Skip("Symlink creation not supported: ", err)
		return
	}

	adminSess := &core.UserSession{
		ID:       "admin-1",
		Username: "admin",
		Type:     "admin",
		IsActive: true,
	}

	// Request the symlinked path.
	req := httptest.NewRequest("GET", "/api/filesystem?path="+filepath.ToSlash(symlinkPath), nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSess))
	rr := httptest.NewRecorder()

	handleGetFilesystem(appRoot).ServeHTTP(rr, req)

	t.Logf("Symlink bypass HTTP Status: %d", rr.Code)
	t.Logf("Symlink bypass Body: %s", rr.Body.String())

	if rr.Code == http.StatusOK {
		var response struct {
			Posix       bool            `json:"posix"`
			Directories []DirectoryInfo `json:"directories"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err == nil {
			t.Logf("Directories listed under symlink: %+v", response.Directories)
			for _, d := range response.Directories {
				if d.Dirname == "secret-sub" {
					t.Logf("SECURITY WARNING: Bypassed metadata exclusion via symlink! Listed: %s", d.Path)
				}
			}
		}
	}
}

func TestCheckPathExists_AdversarialTraversals(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tempDir := t.TempDir()
	libraryFolder := filepath.Join(tempDir, "my-library")
	err := os.MkdirAll(libraryFolder, 0755)
	if err != nil {
		t.Fatalf("Failed to create test library folder: %v", err)
	}

	// Insert library folder
	_, err = db.Exec(`INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder-1', ?, 'lib-1')`, libraryFolder)
	if err != nil {
		t.Fatalf("Failed to insert library folder: %v", err)
	}

	userSess := &core.UserSession{
		ID:                 "user-1",
		Username:           "test-user",
		Type:               "user",
		IsActive:           true,
		AccessAllLibraries: true,
	}

	// Define adversarial directories
	adversarialDirs := []string{
		"../my-library",
		"../../my-library",
		"sub/../../../outside",
		"sub/..\\..\\outside",
		"Book\x00Name",
		"Book Name/.",
		"Book Name/..",
	}

	for _, advDir := range adversarialDirs {
		reqBody := `{"directory": "` + strings.ReplaceAll(advDir, `\`, `\\`) + `", "folderPath": "` + filepath.ToSlash(libraryFolder) + `"}`
		req := httptest.NewRequest("POST", "/api/filesystem/pathexists", bytes.NewBufferString(reqBody))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handleCheckPathExists(db).ServeHTTP(rr, req)
		t.Logf("Checking adversarial directory %q: status %d, body %s", advDir, rr.Code, strings.TrimSpace(rr.Body.String()))
	}
}

func TestCheckPathExists_ConcurrencyStress(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tempDir := t.TempDir()
	libraryFolder := filepath.Join(tempDir, "my-library")
	err := os.MkdirAll(libraryFolder, 0755)
	if err != nil {
		t.Fatalf("Failed to create test library folder: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder-1', ?, 'lib-1')`, libraryFolder)
	if err != nil {
		t.Fatalf("Failed to insert library folder: %v", err)
	}

	userSess := &core.UserSession{
		ID:                 "user-1",
		Username:           "test-user",
		Type:               "user",
		IsActive:           true,
		AccessAllLibraries: true,
	}

	handler := handleCheckPathExists(db)
	reqBody := `{"directory": "nonexistent", "folderPath": "` + filepath.ToSlash(libraryFolder) + `"}`

	const concurrency = 20
	const iterations = 50
	errChan := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			var err error
			for j := 0; j < iterations; j++ {
				req := httptest.NewRequest("POST", "/api/filesystem/pathexists", bytes.NewBufferString(reqBody))
				req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
				rr := httptest.NewRecorder()
				handler.ServeHTTP(rr, req)
				if rr.Code != http.StatusOK {
					err = fmt.Errorf("expected 200 OK, got %d", rr.Code)
					break
				}
			}
			errChan <- err
		}()
	}

	for i := 0; i < concurrency; i++ {
		if err := <-errChan; err != nil {
			t.Error(err)
		}
	}
}

func TestGetFilesystem_ConcurrencyStress(t *testing.T) {
	appRoot := t.TempDir()
	safeDir := filepath.Join(appRoot, "safe-directory")
	err := os.MkdirAll(safeDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create safe dir: %v", err)
	}

	adminSess := &core.UserSession{
		ID:       "admin-1",
		Username: "admin",
		Type:     "admin",
		IsActive: true,
	}

	handler := handleGetFilesystem(appRoot)
	reqPath := filepath.ToSlash(safeDir)

	const concurrency = 20
	const iterations = 50
	errChan := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			var err error
			for j := 0; j < iterations; j++ {
				req := httptest.NewRequest("GET", "/api/filesystem?path="+reqPath, nil)
				req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSess))
				rr := httptest.NewRecorder()
				handler.ServeHTTP(rr, req)
				if rr.Code != http.StatusOK {
					err = fmt.Errorf("expected 200 OK, got %d", rr.Code)
					break
				}
			}
			errChan <- err
		}()
	}

	for i := 0; i < concurrency; i++ {
		if err := <-errChan; err != nil {
			t.Error(err)
		}
	}
}

func BenchmarkCheckPathExists(b *testing.B) {
	db := setupTestDB(b)
	defer db.Close()

	tempDir := b.TempDir()
	libraryFolder := filepath.Join(tempDir, "my-library")
	_ = os.MkdirAll(libraryFolder, 0755)

	_, _ = db.Exec(`INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder-1', ?, 'lib-1')`, libraryFolder)

	userSess := &core.UserSession{
		ID:                 "user-1",
		Username:           "test-user",
		Type:               "user",
		IsActive:           true,
		AccessAllLibraries: true,
	}

	reqBody := `{"directory": "nonexistent", "folderPath": "` + filepath.ToSlash(libraryFolder) + `"}`
	handler := handleCheckPathExists(db)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest("POST", "/api/filesystem/pathexists", bytes.NewBufferString(reqBody))
			req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
		}
	})
}

func BenchmarkGetFilesystem(b *testing.B) {
	appRoot := b.TempDir()
	safeDir := filepath.Join(appRoot, "safe-directory")
	_ = os.MkdirAll(safeDir, 0755)

	adminSess := &core.UserSession{
		ID:       "admin-1",
		Username: "admin",
		Type:     "admin",
		IsActive: true,
	}

	handler := handleGetFilesystem(appRoot)
	reqPath := filepath.ToSlash(safeDir)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest("GET", "/api/filesystem?path="+reqPath, nil)
			req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSess))
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
		}
	})
}
