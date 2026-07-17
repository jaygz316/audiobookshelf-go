package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"audiobookshelf/internal/core"
)

func TestApiKeys_ConcurrencyAndStress(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Pre-insert multiple admin/root users to use in parallel requests
	const numUsers = 5
	for i := 1; i <= numUsers; i++ {
		userID := fmt.Sprintf("user-%d", i)
		username := fmt.Sprintf("admin-%d", i)
		_, err := db.Exec(`INSERT INTO users (id, username, type, isActive, permissions) VALUES (?, ?, 'admin', 1, '{}')`, userID, username)
		if err != nil {
			t.Fatalf("Failed to insert user %s: %v", userID, err)
		}
	}

	// Create user sessions for auth mock
	var sessions []*core.UserSession
	for i := 1; i <= numUsers; i++ {
		sessions = append(sessions, &core.UserSession{
			ID:       fmt.Sprintf("user-%d", i),
			Username: fmt.Sprintf("admin-%d", i),
			Type:     "admin",
			IsActive: true,
		})
	}

	var wg sync.WaitGroup
	const concurrency = 50
	const opsPerGoroutine = 20

	// Channels to track active API keys generated during the test
	keyChan := make(chan string, concurrency*opsPerGoroutine)

	// Start concurrent workers
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			// Select a user session for this worker to round-robin
			session := sessions[workerID%numUsers]

			for j := 0; j < opsPerGoroutine; j++ {
				op := (workerID + j) % 4
				switch op {
				case 0:
					// Scenario 1: Create an API key
					reqPayload := CreateApiKeyRequest{
						Name:      fmt.Sprintf("Key-W%d-O%d", workerID, j),
						UserID:    session.ID,
						ExpiresAt: time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339),
					}
					body, err := json.Marshal(reqPayload)
					if err != nil {
						t.Errorf("worker %d: marshal err: %v", workerID, err)
						continue
					}
					req := httptest.NewRequest("POST", "/api/api-keys", bytes.NewReader(body))
					req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, session))
					rr := httptest.NewRecorder()

					handlePostApiKey(db).ServeHTTP(rr, req)

					if rr.Code != http.StatusOK {
						t.Errorf("worker %d: expected POST status 200, got %d. Body: %s", workerID, rr.Code, rr.Body.String())
						continue
					}

					var resp map[string]interface{}
					if err := json.NewDecoder(rr.Body).Decode(&resp); err == nil {
						if apiKey, ok := resp["apiKey"].(map[string]interface{}); ok {
							if token, ok := apiKey["token"].(string); ok {
								keyChan <- token
							}
						}
					}

				case 1:
					// Scenario 2: Get API keys
					req := httptest.NewRequest("GET", "/api/api-keys", nil)
					req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, session))
					rr := httptest.NewRecorder()

					handleGetApiKeys(db).ServeHTTP(rr, req)

					if rr.Code != http.StatusOK {
						t.Errorf("worker %d: expected GET status 200, got %d", workerID, rr.Code)
					}

				case 2:
					// Scenario 3: Validation / AuthMiddleware check (if a key exists)
					select {
					case token := <-keyChan:
						// Try to authenticate using this token
						nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							w.WriteHeader(http.StatusOK)
						})
						mw := AuthMiddleware(db, "secret", nextHandler)

						reqAuth := httptest.NewRequest("GET", "/api/me", nil)
						reqAuth.Header.Set("Authorization", "Bearer "+token)
						rrAuth := httptest.NewRecorder()

						mw.ServeHTTP(rrAuth, reqAuth)

						// Re-insert token back to let others use it, unless it's deleted
						// Check status: might be 200 OK (if valid) or 401 Unauthorized (if deleted concurrently)
						if rrAuth.Code != http.StatusOK && rrAuth.Code != http.StatusUnauthorized {
							t.Errorf("worker %d: unexpected Auth status: %d", workerID, rrAuth.Code)
						}

						// Put it back for someone else to delete or validate
						keyChan <- token
					default:
						// No keys generated yet, sleep briefly
						time.Sleep(1 * time.Millisecond)
					}

				case 3:
					// Scenario 4: Delete API key
					select {
					case token := <-keyChan:
						req := httptest.NewRequest("DELETE", "/api/api-keys/"+token, nil)
						req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, session))
						rr := httptest.NewRecorder()

						handleDeleteApiKey(db).ServeHTTP(rr, req)

						if rr.Code != http.StatusOK {
							t.Errorf("worker %d: expected DELETE status 200, got %d", workerID, rr.Code)
						}
					default:
						// No keys generated yet, sleep briefly
						time.Sleep(1 * time.Millisecond)
					}
				}
			}
		}(i)
	}

	wg.Wait()
	close(keyChan)
}
