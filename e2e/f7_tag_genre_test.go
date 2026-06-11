package e2e

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

func TestF7TagsGenres(t *testing.T) {
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

	// 2. Setup Normal User (Non-Admin)
	hashedPash, err := bcrypt.GenerateFromPassword([]byte("normalpassword123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}
	db, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	permsJSON := `{"download":true,"accessExplicitContent":false,"accessAllLibraries":true,"librariesAccessible":[],"accessAllTags":true,"itemTagsSelected":[],"selectedTagsNotAccessible":false}`
	_, err = db.Exec(`INSERT INTO users (id, username, email, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, ?, ?, 1, ?, '{}', '[]', datetime('now'), datetime('now'))`,
		uuid.New().String(), "normaluser", "normal@test.com", "user", string(hashedPash), "token-normal", permsJSON)
	if err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}

	// Login as normal user
	normalLoginPayload := map[string]string{
		"username": "normaluser",
		"password": "normalpassword123",
	}
	normalLoginBody, _ := json.Marshal(normalLoginPayload)
	resp, err = client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(normalLoginBody))
	if err != nil {
		t.Fatalf("Failed to login normal user: %v", err)
	}
	var normalResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&normalResp)
	resp.Body.Close()
	normalToken := normalResp["user"].(map[string]interface{})["accessToken"].(string)

	// 3. Seed Books & Podcasts with Tags and Genres
	_, err = db.Exec(`INSERT INTO books (id, title, tags, genres) VALUES (?, ?, ?, ?)`,
		"book_1", "Test Book 1", `["Fiction", "Sci-Fi", "Adventure"]`, `["Classic", "Thriller"]`)
	if err != nil {
		t.Fatalf("Seeding book_1 failed: %v", err)
	}
	_, err = db.Exec(`INSERT INTO books (id, title, tags, genres) VALUES (?, ?, ?, ?)`,
		"book_2", "Test Book 2", `["sci-fi", "Mystery"]`, `["classic", "horror"]`)
	if err != nil {
		t.Fatalf("Seeding book_2 failed: %v", err)
	}
	_, err = db.Exec(`INSERT INTO podcasts (id, title, tags, genres) VALUES (?, ?, ?, ?)`,
		"podcast_1", "Test Podcast 1", `["history", "Adventure"]`, `["Education", "Thriller"]`)
	if err != nil {
		t.Fatalf("Seeding podcast_1 failed: %v", err)
	}

	// Close DB connection so app can write without locking
	db.Close()

	// --- Tier 1 Tests ---

	// 1. List tags (GET /api/tags)
	t.Run("GET /api/tags - success", func(t *testing.T) {
		req, _ := http.NewRequest("GET", h.BaseURL+"/api/tags", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200, got %d", resp.StatusCode)
		}

		var data map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&data)
		tagsInterface, ok := data["tags"].([]interface{})
		if !ok {
			t.Fatalf("Invalid response structure: %v", data)
		}

		// Convert to slice of strings
		tags := []string{}
		for _, tagVal := range tagsInterface {
			tags = append(tags, tagVal.(string))
		}

		// Verify unique tags were retrieved (Fiction, Sci-Fi, Adventure, sci-fi, Mystery, history)
		expectedTags := map[string]bool{
			"Fiction":   true,
			"Sci-Fi":    true,
			"Adventure": true,
			"sci-fi":    true,
			"Mystery":   true,
			"history":   true,
		}

		for _, tag := range tags {
			delete(expectedTags, tag)
		}

		if len(expectedTags) > 0 {
			t.Errorf("Missing expected tags: %v", expectedTags)
		}
	})

	// 2. List genres (GET /api/genres)
	t.Run("GET /api/genres - success", func(t *testing.T) {
		req, _ := http.NewRequest("GET", h.BaseURL+"/api/genres", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200, got %d", resp.StatusCode)
		}

		var data map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&data)
		genresInterface, ok := data["genres"].([]interface{})
		if !ok {
			t.Fatalf("Invalid response structure: %v", data)
		}

		genres := []string{}
		for _, gVal := range genresInterface {
			genres = append(genres, gVal.(string))
		}

		expectedGenres := map[string]bool{
			"Classic":   true,
			"Thriller":  true,
			"classic":   true,
			"horror":    true,
			"Education": true,
		}

		for _, genre := range genres {
			delete(expectedGenres, genre)
		}

		if len(expectedGenres) > 0 {
			t.Errorf("Missing expected genres: %v", expectedGenres)
		}
	})

	// 3. Rename tag (POST /api/tags/rename)
	t.Run("POST /api/tags/rename - success", func(t *testing.T) {
		payload := map[string]string{
			"tag":    "Sci-Fi",
			"newTag": "Science Fiction",
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", h.BaseURL+"/api/tags/rename", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200, got %d", resp.StatusCode)
		}

		// Verify tag renamed in list
		reqList, _ := http.NewRequest("GET", h.BaseURL+"/api/tags", nil)
		reqList.Header.Set("Authorization", "Bearer "+adminToken)
		respList, err := client.Do(reqList)
		if err != nil {
			t.Fatalf("Failed GET tags: %v", err)
		}
		defer respList.Body.Close()

		var data map[string]interface{}
		json.NewDecoder(respList.Body).Decode(&data)
		tags := data["tags"].([]interface{})

		foundOld := false
		foundNew := false
		for _, tVal := range tags {
			s := tVal.(string)
			if s == "Sci-Fi" {
				foundOld = true
			}
			if s == "Science Fiction" {
				foundNew = true
			}
		}

		if foundOld {
			t.Errorf("Old tag Sci-Fi still exists")
		}
		if !foundNew {
			t.Errorf("New tag Science Fiction not found")
		}
	})

	// 4. Rename genre (POST /api/genres/rename)
	t.Run("POST /api/genres/rename - success", func(t *testing.T) {
		payload := map[string]string{
			"genre":    "Classic",
			"newGenre": "Classics",
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", h.BaseURL+"/api/genres/rename", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200, got %d", resp.StatusCode)
		}

		// Verify genre renamed in list
		reqList, _ := http.NewRequest("GET", h.BaseURL+"/api/genres", nil)
		reqList.Header.Set("Authorization", "Bearer "+adminToken)
		respList, err := client.Do(reqList)
		if err != nil {
			t.Fatalf("Failed GET genres: %v", err)
		}
		defer respList.Body.Close()

		var data map[string]interface{}
		json.NewDecoder(respList.Body).Decode(&data)
		genres := data["genres"].([]interface{})

		foundOld := false
		foundNew := false
		for _, gVal := range genres {
			s := gVal.(string)
			if s == "Classic" {
				foundOld = true
			}
			if s == "Classics" {
				foundNew = true
			}
		}

		if foundOld {
			t.Errorf("Old genre Classic still exists")
		}
		if !foundNew {
			t.Errorf("New genre Classics not found")
		}
	})

	// 5. Delete tag (DELETE /api/tags/:base64EncodedTagName)
	t.Run("DELETE /api/tags/:base64EncodedTagName - success", func(t *testing.T) {
		tagToDelete := "Adventure"
		b64Tag := base64.StdEncoding.EncodeToString([]byte(tagToDelete))

		req, _ := http.NewRequest("DELETE", h.BaseURL+"/api/tags/"+b64Tag, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200, got %d", resp.StatusCode)
		}

		// Verify tag deleted in list
		reqList, _ := http.NewRequest("GET", h.BaseURL+"/api/tags", nil)
		reqList.Header.Set("Authorization", "Bearer "+adminToken)
		respList, err := client.Do(reqList)
		if err != nil {
			t.Fatalf("Failed GET tags: %v", err)
		}
		defer respList.Body.Close()

		var data map[string]interface{}
		json.NewDecoder(respList.Body).Decode(&data)
		tags := data["tags"].([]interface{})

		for _, tVal := range tags {
			if tVal.(string) == tagToDelete {
				t.Errorf("Deleted tag %s still exists in tags list", tagToDelete)
			}
		}
	})

	// 6. Delete genre (DELETE /api/genres/:base64EncodedGenreName)
	t.Run("DELETE /api/genres/:base64EncodedGenreName - success", func(t *testing.T) {
		genreToDelete := "Thriller"
		b64Genre := base64.StdEncoding.EncodeToString([]byte(genreToDelete))

		req, _ := http.NewRequest("DELETE", h.BaseURL+"/api/genres/"+b64Genre, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200, got %d", resp.StatusCode)
		}

		// Verify genre deleted in list
		reqList, _ := http.NewRequest("GET", h.BaseURL+"/api/genres", nil)
		reqList.Header.Set("Authorization", "Bearer "+adminToken)
		respList, err := client.Do(reqList)
		if err != nil {
			t.Fatalf("Failed GET genres: %v", err)
		}
		defer respList.Body.Close()

		var data map[string]interface{}
		json.NewDecoder(respList.Body).Decode(&data)
		genres := data["genres"].([]interface{})

		for _, gVal := range genres {
			if gVal.(string) == genreToDelete {
				t.Errorf("Deleted genre %s still exists in genres list", genreToDelete)
			}
		}
	})

	// --- Tier 2 Tests ---

	// 7. Access control check (non-admin/normal user blocked from renaming/deleting tags and genres with HTTP 403)
	t.Run("Access control - normal user blocked", func(t *testing.T) {
		b64Val := base64.StdEncoding.EncodeToString([]byte("TestTag"))
		endpoints := []struct {
			method string
			url    string
			body   io.Reader
		}{
			{"POST", h.BaseURL + "/api/tags/rename", bytes.NewReader([]byte(`{"tag":"Fiction","newTag":"Fic"}`))},
			{"POST", h.BaseURL + "/api/genres/rename", bytes.NewReader([]byte(`{"genre":"horror","newGenre":"scary"}`))},
			{"DELETE", h.BaseURL + "/api/tags/" + b64Val, nil},
			{"DELETE", h.BaseURL + "/api/genres/" + b64Val, nil},
		}

		for _, ep := range endpoints {
			req, _ := http.NewRequest(ep.method, ep.url, ep.body)
			req.Header.Set("Authorization", "Bearer "+normalToken)
			if ep.body != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Errorf("Request to %s %s failed: %v", ep.method, ep.url, err)
				continue
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("Expected 403 Forbidden for %s %s, got %d", ep.method, ep.url, resp.StatusCode)
			}
		}
	})

	// 8. Delete non-existent tag behaves gracefully (returns HTTP 200 OK)
	t.Run("Delete non-existent tag", func(t *testing.T) {
		b64NonExistent := base64.StdEncoding.EncodeToString([]byte("NonExistentTag123"))
		req, _ := http.NewRequest("DELETE", h.BaseURL+"/api/tags/"+b64NonExistent, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 OK for deleting non-existent tag, got %d", resp.StatusCode)
		}
	})

	// 9. Delete non-existent genre behaves gracefully (returns HTTP 200 OK)
	t.Run("Delete non-existent genre", func(t *testing.T) {
		b64NonExistent := base64.StdEncoding.EncodeToString([]byte("NonExistentGenre123"))
		req, _ := http.NewRequest("DELETE", h.BaseURL+"/api/genres/"+b64NonExistent, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 OK for deleting non-existent genre, got %d", resp.StatusCode)
		}
	})

	// 10. Rename tag/genre with missing new tag/genre or invalid input (HTTP 400 Bad Request)
	t.Run("Rename validation errors", func(t *testing.T) {
		invalidInputs := []struct {
			endpoint string
			payload  string
		}{
			{"/api/tags/rename", `{"tag": ""}`},
			{"/api/tags/rename", `{"newTag": "Something"}`},
			{"/api/tags/rename", `{"tag": "Fiction", "newTag": ""}`},
			{"/api/tags/rename", `invalid-json`},
			{"/api/genres/rename", `{"genre": ""}`},
			{"/api/genres/rename", `{"newGenre": "Something"}`},
			{"/api/genres/rename", `{"genre": "Classic", "newGenre": ""}`},
			{"/api/genres/rename", `invalid-json`},
		}

		for _, inp := range invalidInputs {
			req, _ := http.NewRequest("POST", h.BaseURL+inp.endpoint, strings.NewReader(inp.payload))
			req.Header.Set("Authorization", "Bearer "+adminToken)
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("Expected 400 Bad Request for %s with payload %s, got %d", inp.endpoint, inp.payload, resp.StatusCode)
			}
		}
	})

	// 11. Case-insensitivity verification (list tags/genres should sort alphabetically, case-insensitively)
	t.Run("Case-insensitivity verification", func(t *testing.T) {
		// Verify tags alphabetical ordering
		reqTags, _ := http.NewRequest("GET", h.BaseURL+"/api/tags", nil)
		reqTags.Header.Set("Authorization", "Bearer "+adminToken)
		respTags, err := client.Do(reqTags)
		if err != nil {
			t.Fatalf("Failed request: %v", err)
		}
		defer respTags.Body.Close()

		var dataTags map[string]interface{}
		json.NewDecoder(respTags.Body).Decode(&dataTags)
		tagsList := dataTags["tags"].([]interface{})

		for i := 0; i < len(tagsList)-1; i++ {
			s1 := strings.ToLower(tagsList[i].(string))
			s2 := strings.ToLower(tagsList[i+1].(string))
			if s1 > s2 {
				t.Errorf("Tags are not sorted case-insensitively: %s came before %s", tagsList[i], tagsList[i+1])
			}
		}

		// Verify genres alphabetical ordering
		reqGenres, _ := http.NewRequest("GET", h.BaseURL+"/api/genres", nil)
		reqGenres.Header.Set("Authorization", "Bearer "+adminToken)
		respGenres, err := client.Do(reqGenres)
		if err != nil {
			t.Fatalf("Failed request: %v", err)
		}
		defer respGenres.Body.Close()

		var dataGenres map[string]interface{}
		json.NewDecoder(respGenres.Body).Decode(&dataGenres)
		genresList := dataGenres["genres"].([]interface{})

		for i := 0; i < len(genresList)-1; i++ {
			s1 := strings.ToLower(genresList[i].(string))
			s2 := strings.ToLower(genresList[i+1].(string))
			if s1 > s2 {
				t.Errorf("Genres are not sorted case-insensitively: %s came before %s", genresList[i], genresList[i+1])
			}
		}
	})
}
