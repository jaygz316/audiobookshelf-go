package hls

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupStressTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open memory db: %v", err)
	}

	queries := []string{
		`CREATE TABLE libraryFolders (id TEXT PRIMARY KEY, path TEXT, libraryId TEXT)`,
		`CREATE TABLE playbackSessions (id TEXT PRIMARY KEY, userId TEXT, mediaItemId TEXT, mediaItemType TEXT, startTime REAL, libraryId TEXT, extraData TEXT)`,
		`CREATE TABLE books (id TEXT PRIMARY KEY, title TEXT, audioFiles BLOB)`,
		`CREATE TABLE podcastEpisodes (id TEXT PRIMARY KEY, podcastId TEXT, title TEXT, audioFile TEXT)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("Failed to execute query %q: %v", q, err)
		}
	}

	// Insert standard seed
	_, _ = db.Exec(`INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder-1', '/fake', 'lib-1')`)

	return db
}

func TestStreamManager_Stress(t *testing.T) {
	db := setupStressTestDB(t)
	defer db.Close()

	// Seed some playback sessions and books
	numSessions := 20
	for i := 0; i < numSessions; i++ {
		sessID := fmt.Sprintf("session-%d", i)
		bookID := fmt.Sprintf("book-%d", i)
		audioFilesJSON := fmt.Sprintf(`[
			{"index":0, "exclude":false, "duration":100.0, "codec":"mp3", "mimeType":"audio/mpeg", "metadata":{"path":"/fake/path-%d.mp3"}}
		]`, i)

		_, err := db.Exec(`INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, extraData) 
			VALUES (?, 'user-1', ?, 'book', 0.0, '{"libraryItemId":"item-1"}')`, sessID, bookID)
		if err != nil {
			t.Fatalf("seed session failed: %v", err)
		}

		_, err = db.Exec(`INSERT INTO books (id, title, audioFiles) VALUES (?, 'Fake Book', ?)`, bookID, audioFilesJSON)
		if err != nil {
			t.Fatalf("seed book failed: %v", err)
		}
	}

	sm := NewStreamManager()
	tempDir, err := os.MkdirTemp("", "hls-sm-stress-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// We will run concurrent workers loading, retrieving, and removing streams
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Worker 1: LoadOrCreateStream concurrently
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					sessID := fmt.Sprintf("session-%d", rand.Intn(numSessions))
					s, err := sm.LoadOrCreateStream(db, sessID, tempDir, nil)
					if err == nil && s != nil {
						_ = sm.GetStream(sessID)
						if rand.Float64() < 0.2 {
							sm.RemoveStream(sessID)
						}
					} else if err != nil {
						// Since ffmpeg is likely not present or might fail, we ignore start transcode failures
						if !strings.Contains(err.Error(), "exec") && !strings.Contains(err.Error(), "failed to start transcode") {
							// For other unexpected errors, we could log or report them, but ignore start failures.
						}
					}
					time.Sleep(1 * time.Millisecond)
				}
			}
		}()
	}

	// Worker 2: Randomly calling Close() or RemoveStream
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if rand.Float64() < 0.1 {
					sm.Close()
				} else {
					sessID := fmt.Sprintf("session-%d", rand.Intn(numSessions))
					sm.RemoveStream(sessID)
				}
				time.Sleep(5 * time.Millisecond)
			}
		}
	}()

	wg.Wait()
	sm.Close()
}
