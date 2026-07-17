package podcast

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	_ "modernc.org/sqlite"
)

var dbCounterRetention int64

func setupRetentionTestDB(t *testing.T) *sql.DB {
	id := atomic.AddInt64(&dbCounterRetention, 1)
	dsn := fmt.Sprintf("file:retentionmemdb%d?mode=memory&cache=shared", id)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	db.SetMaxIdleConns(2)

	schemas := []string{
		`CREATE TABLE podcasts (
			id TEXT PRIMARY KEY,
			title TEXT,
			feedURL TEXT,
			autoDownloadEpisodes INTEGER,
			maxEpisodesToKeep INTEGER,
			maxNewEpisodesToDownload INTEGER,
			autoDeletePlayed INTEGER
		)`,
		`CREATE TABLE podcastEpisodes (
			id TEXT PRIMARY KEY,
			podcastId TEXT,
			title TEXT,
			audioFile TEXT,
			pubDate TEXT,
			publishedAt TEXT,
			createdAt TEXT
		)`,
	}

	for _, schema := range schemas {
		if _, err := db.Exec(schema); err != nil {
			_ = db.Close()
			t.Fatalf("failed to create schema: %v", err)
		}
	}
	return db
}

func TestEnforceRetentionPolicy(t *testing.T) {
	db := setupRetentionTestDB(t)
	defer db.Close()
	m := NewPodcastManager(db)

	tempDir := t.TempDir()

	createDummyEpisodeFile := func(name string) (string, string) {
		path := filepath.Join(tempDir, name)
		err := os.WriteFile(path, []byte("audio-content"), 0644)
		if err != nil {
			t.Fatalf("failed to write dummy episode: %v", err)
		}
		audioFileObj := map[string]interface{}{
			"metadata": map[string]interface{}{
				"path": path,
			},
		}
		b, err := json.Marshal(audioFileObj)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}
		return path, string(b)
	}

	p0, json0 := createDummyEpisodeFile("ep0.mp3")
	p1, json1 := createDummyEpisodeFile("ep1.mp3")
	p2, json2 := createDummyEpisodeFile("ep2.mp3")
	p3, json3 := createDummyEpisodeFile("ep3.mp3")
	p4, json4 := createDummyEpisodeFile("ep4.mp3")

	podID := "test-pod-retention"

	episodes := []struct {
		id          string
		title       string
		audioFile   string
		pubDate     string
		publishedAt string
		createdAt   string
	}{
		{"e4", "Ep4", json4, "2026-07-08", "2026-07-08T10:00:00Z", "2026-07-08 09:00:00"},
		{"e1", "Ep1", json1, "2026-07-09", "", ""},
		{"e3", "Ep3", json3, "2026-07-08", "2026-07-08T10:00:00Z", "2026-07-08 10:00:00"},
		{"e0", "Ep0", json0, "2026-07-10", "", ""},
		{"e2", "Ep2", json2, "2026-07-08", "2026-07-08T12:00:00Z", ""},
	}

	for _, ep := range episodes {
		_, err := db.Exec(`
			INSERT INTO podcastEpisodes (id, podcastId, title, audioFile, pubDate, publishedAt, createdAt)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, ep.id, podID, ep.title, ep.audioFile, ep.pubDate, ep.publishedAt, ep.createdAt)
		if err != nil {
			t.Fatalf("failed to insert episode: %v", err)
		}
	}

	ctx := context.Background()
	err := m.EnforceRetentionPolicy(ctx, podID, 3)
	if err != nil {
		t.Fatalf("EnforceRetentionPolicy failed: %v", err)
	}

	exists := func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}

	if !exists(p0) || !exists(p1) || !exists(p2) {
		t.Error("expected newest 3 episodes (e0, e1, e2) to remain on disk")
	}
	if exists(p3) || exists(p4) {
		t.Error("expected oldest 2 episodes (e3, e4) to be deleted from disk")
	}

	checkDB := func(id string, shouldHaveAudio bool) {
		var audioFile string
		err := db.QueryRow("SELECT audioFile FROM podcastEpisodes WHERE id = ?", id).Scan(&audioFile)
		if err != nil {
			t.Fatalf("query failed for %s: %v", id, err)
		}
		if shouldHaveAudio {
			if audioFile == "{}" || audioFile == "" {
				t.Errorf("expected episode %s to have audioFile in DB, got %q", id, audioFile)
			}
		} else {
			if audioFile != "{}" {
				t.Errorf("expected episode %s to have empty/cleared audioFile in DB, got %q", id, audioFile)
			}
		}
	}

	checkDB("e0", true)
	checkDB("e1", true)
	checkDB("e2", true)
	checkDB("e3", false)
	checkDB("e4", false)
}

func TestEnforceRetentionPolicy_Noop(t *testing.T) {
	db := setupRetentionTestDB(t)
	defer db.Close()
	m := NewPodcastManager(db)
	tempDir := t.TempDir()

	path := filepath.Join(tempDir, "noop.mp3")
	_ = os.WriteFile(path, []byte("data"), 0644)
	audioFile := fmt.Sprintf(`{"metadata": {"path": "%s"}}`, path)

	podID := "noop-pod"
	_, _ = db.Exec(`INSERT INTO podcastEpisodes (id, podcastId, title, audioFile) VALUES (?, ?, ?, ?)`, "noop-ep", podID, "Noop Ep", audioFile)

	_ = m.EnforceRetentionPolicy(context.Background(), podID, 0)
	if _, err := os.Stat(path); err != nil {
		t.Error("expected file to not be deleted when maxKeep <= 0")
	}

	_ = m.EnforceRetentionPolicy(context.Background(), podID, 5)
	if _, err := os.Stat(path); err != nil {
		t.Error("expected file to not be deleted when maxKeep > total downloads")
	}
}
