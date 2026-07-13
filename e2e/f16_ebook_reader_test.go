package e2e

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestF16EbookReader(t *testing.T) {
	h := NewTestHarness()
	if err := h.Start(); err != nil {
		t.Fatalf("Failed to start harness: %v", err)
	}
	defer h.Stop()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("Failed to create cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}

	// 1. Setup Admin Root & login
	initPayload := map[string]interface{}{
		"newRoot": map[string]string{
			"username": "rootadmin",
			"password": "supersecurepassword123",
		},
	}
	initBody, _ := json.Marshal(initPayload)
	resp, err := client.Post(h.BaseURL+"/init", "application/json", bytes.NewReader(initBody))
	if err != nil {
		t.Fatalf("Failed to initialize root: %v", err)
	}
	resp.Body.Close()

	loginPayload := map[string]string{
		"username": "rootadmin",
		"password": "supersecurepassword123",
	}
	loginBody, _ := json.Marshal(loginPayload)
	resp, err = client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatalf("Failed to login admin: %v", err)
	}
	var adminResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&adminResp)
	resp.Body.Close()
	adminToken := adminResp["user"].(map[string]interface{})["accessToken"].(string)

	// 2. Write fake EPUB and PDF files to test paths
	tempDir := t.TempDir()

	fakeEpubPath := filepath.Join(tempDir, "test.epub")
	epubContent := []byte("fake epub data")
	if err := os.WriteFile(fakeEpubPath, epubContent, 0644); err != nil {
		t.Fatalf("Failed to write fake epub file: %v", err)
	}

	fakePdfPath := filepath.Join(tempDir, "test.pdf")
	pdfContent := []byte("fake pdf data")
	if err := os.WriteFile(fakePdfPath, pdfContent, 0644); err != nil {
		t.Fatalf("Failed to write fake pdf file: %v", err)
	}

	// 3. Open DB to insert libraries, books, and items
	db, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('lib-ebook', 'Ebooks Library', 'book')`)
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}

	// Insert EPUB Book
	epubEbookJSON := `{"ebookFormat":"epub", "metadata":{"filename":"test.epub", "ext":".epub", "path":"` + filepath.ToSlash(fakeEpubPath) + `", "size":14}}`
	_, err = db.Exec(`INSERT INTO books (id, title, duration, coverPath, narrators, audioFiles, ebookFile, chapters, tags, genres) VALUES 
		('book-epub', 'EPUB Test Book', 0, '', '[]', '[]', ?, '[]', '[]', '[]')`, epubEbookJSON)
	if err != nil {
		t.Fatalf("Failed to insert epub book: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title) VALUES ('item-epub', 'lib-ebook', 'book', 'book-epub', 'EPUB Test Book')`)
	if err != nil {
		t.Fatalf("Failed to insert epub libraryItem: %v", err)
	}

	// Insert PDF Book
	pdfEbookJSON := `{"ebookFormat":"pdf", "metadata":{"filename":"test.pdf", "ext":".pdf", "path":"` + filepath.ToSlash(fakePdfPath) + `", "size":13}}`
	_, err = db.Exec(`INSERT INTO books (id, title, duration, coverPath, narrators, audioFiles, ebookFile, chapters, tags, genres) VALUES 
		('book-pdf', 'PDF Test Book', 0, '', '[]', '[]', ?, '[]', '[]', '[]')`, pdfEbookJSON)
	if err != nil {
		t.Fatalf("Failed to insert pdf book: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title) VALUES ('item-pdf', 'lib-ebook', 'book', 'book-pdf', 'PDF Test Book')`)
	if err != nil {
		t.Fatalf("Failed to insert pdf libraryItem: %v", err)
	}

	// 4. Test Serving EPUB Ebook
	t.Run("Serve EPUB Ebook", func(t *testing.T) {
		req, _ := http.NewRequest("GET", h.BaseURL+"/api/items/item-epub/ebook", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("GET ebook failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		if cType := resp.Header.Get("Content-Type"); cType != "application/epub+zip" {
			t.Errorf("Expected Content-Type application/epub+zip, got %q", cType)
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		if string(bodyBytes) != string(epubContent) {
			t.Errorf("Expected body %q, got %q", epubContent, bodyBytes)
		}
	})

	// 5. Test Serving PDF Ebook
	t.Run("Serve PDF Ebook", func(t *testing.T) {
		req, _ := http.NewRequest("GET", h.BaseURL+"/api/items/item-pdf/ebook", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("GET ebook failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		if cType := resp.Header.Get("Content-Type"); cType != "application/pdf" {
			t.Errorf("Expected Content-Type application/pdf, got %q", cType)
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		if string(bodyBytes) != string(pdfContent) {
			t.Errorf("Expected body %q, got %q", pdfContent, bodyBytes)
		}
	})

	// 6. Test Ebook Progress Synchronization
	t.Run("Sync and Get Ebook Progress", func(t *testing.T) {
		// Verify initial GET returns 404 since no progress has been created yet
		reqGet, _ := http.NewRequest("GET", h.BaseURL+"/api/me/progress/item-epub", nil)
		reqGet.Header.Set("Authorization", "Bearer "+adminToken)
		respGet, err := client.Do(reqGet)
		if err != nil {
			t.Fatalf("GET progress failed: %v", err)
		}
		respGet.Body.Close()
		if respGet.StatusCode != http.StatusNotFound {
			t.Errorf("Expected initial progress GET to return 404, got %d", respGet.StatusCode)
		}

		// Save progress via PATCH
		progressPayload := map[string]interface{}{
			"ebookLocation": "epubcfi(/6/12[chap-4]!/4/2/10/1:0)",
			"ebookProgress": 0.35,
		}
		progressBody, _ := json.Marshal(progressPayload)
		reqPatch, _ := http.NewRequest("PATCH", h.BaseURL+"/api/me/progress/item-epub", bytes.NewReader(progressBody))
		reqPatch.Header.Set("Authorization", "Bearer "+adminToken)
		reqPatch.Header.Set("Content-Type", "application/json")

		respPatch, err := client.Do(reqPatch)
		if err != nil {
			t.Fatalf("PATCH progress failed: %v", err)
		}
		defer respPatch.Body.Close()

		if respPatch.StatusCode != http.StatusOK {
			t.Fatalf("Expected PATCH progress status 200, got %d", respPatch.StatusCode)
		}

		// Retrieve progress via GET
		reqGet2, _ := http.NewRequest("GET", h.BaseURL+"/api/me/progress/item-epub", nil)
		reqGet2.Header.Set("Authorization", "Bearer "+adminToken)
		respGet2, err := client.Do(reqGet2)
		if err != nil {
			t.Fatalf("GET progress failed: %v", err)
		}
		defer respGet2.Body.Close()

		if respGet2.StatusCode != http.StatusOK {
			t.Fatalf("Expected GET progress status 200, got %d", respGet2.StatusCode)
		}

		var getProgressResp map[string]interface{}
		json.NewDecoder(respGet2.Body).Decode(&getProgressResp)

		if loc, ok := getProgressResp["ebookLocation"].(string); !ok || loc != "epubcfi(/6/12[chap-4]!/4/2/10/1:0)" {
			t.Errorf("Expected ebookLocation 'epubcfi(/6/12[chap-4]!/4/2/10/1:0)', got %q", getProgressResp["ebookLocation"])
		}

		if prog, ok := getProgressResp["ebookProgress"].(float64); !ok || prog != 0.35 {
			t.Errorf("Expected ebookProgress 0.35, got %v", getProgressResp["ebookProgress"])
		}

		// Update progress with higher progress percentage
		progressPayload2 := map[string]interface{}{
			"ebookLocation": "epubcfi(/6/18[chap-6]!/4/2/20/1:0)",
			"ebookProgress": 0.65,
		}
		progressBody2, _ := json.Marshal(progressPayload2)
		reqPatch2, _ := http.NewRequest("PATCH", h.BaseURL+"/api/me/progress/item-epub", bytes.NewReader(progressBody2))
		reqPatch2.Header.Set("Authorization", "Bearer "+adminToken)
		reqPatch2.Header.Set("Content-Type", "application/json")

		respPatch2, err := client.Do(reqPatch2)
		if err != nil {
			t.Fatalf("PATCH progress update failed: %v", err)
		}
		respPatch2.Body.Close()

		// Get updated progress
		reqGet3, _ := http.NewRequest("GET", h.BaseURL+"/api/me/progress/item-epub", nil)
		reqGet3.Header.Set("Authorization", "Bearer "+adminToken)
		respGet3, err := client.Do(reqGet3)
		if err != nil {
			t.Fatalf("GET updated progress failed: %v", err)
		}
		defer respGet3.Body.Close()

		var getProgressResp2 map[string]interface{}
		json.NewDecoder(respGet3.Body).Decode(&getProgressResp2)

		if loc, ok := getProgressResp2["ebookLocation"].(string); !ok || loc != "epubcfi(/6/18[chap-6]!/4/2/20/1:0)" {
			t.Errorf("Expected updated ebookLocation 'epubcfi(/6/18[chap-6]!/4/2/20/1:0)', got %q", getProgressResp2["ebookLocation"])
		}

		if prog, ok := getProgressResp2["ebookProgress"].(float64); !ok || prog != 0.65 {
			t.Errorf("Expected updated ebookProgress 0.65, got %v", getProgressResp2["ebookProgress"])
		}
	})
}
