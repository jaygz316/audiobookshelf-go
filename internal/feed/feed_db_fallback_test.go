package feed

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"

	_ "modernc.org/sqlite"
)

var fallbackDBCounter int64

func TestPodcastEpisodesFallback(t *testing.T) {
	// Create an in-memory database with a schema that has missing columns in podcastEpisodes
	id := atomic.AddInt64(&fallbackDBCounter, 1)
	dsn := fmt.Sprintf("file:fallbackdb%d?mode=memory&cache=shared", id)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Only create the core columns, omitting pubDate, description, season, episode, episodeType
	schema := `CREATE TABLE podcastEpisodes (
		id TEXT PRIMARY KEY,
		podcastId TEXT,
		title TEXT,
		audioFile TEXT
	)`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create simplified schema: %v", err)
	}

	// Insert mock data
	_, err = db.Exec(`
		INSERT INTO podcastEpisodes (id, podcastId, title, audioFile)
		VALUES (?, ?, ?, ?)
	`, "ep1", "pod1", "Episode 1", `{"duration": 100.0}`)
	if err != nil {
		t.Fatalf("failed to insert simplified episode: %v", err)
	}

	ctx := context.Background()

	// 1. Edge Case: Check hasColumn helper for missing table
	if hasColumn(ctx, db, "nonExistentTable", "id") {
		t.Error("expected hasColumn for nonExistentTable to return false, but got true")
	}

	// 2. Edge Case: Check hasColumn helper for missing column in existing table
	if hasColumn(ctx, db, "podcastEpisodes", "nonExistentColumn") {
		t.Error("expected hasColumn for nonExistentColumn to return false, but got true")
	}

	// 3. Edge Case: Check hasColumn helper for existing column
	if !hasColumn(ctx, db, "podcastEpisodes", "title") {
		t.Error("expected hasColumn for 'title' to return true, but got false")
	}

	// 4. Edge Case: queryPodcastEpisode with non-existent episode ID (should return sql.ErrNoRows wrapped error)
	_, err = queryPodcastEpisode(ctx, db, "nonExistentEp")
	if err == nil {
		t.Error("expected error for non-existent episode, got nil")
	}

	// 5. Test queryPodcastEpisode with valid episode
	ep, err := queryPodcastEpisode(ctx, db, "ep1")
	if err != nil {
		t.Fatalf("queryPodcastEpisode failed: %v", err)
	}
	if ep == nil {
		t.Fatal("expected queryPodcastEpisode to return an episode, got nil")
	}
	if ep.ID != "ep1" || ep.Title != "Episode 1" || ep.AudioFile != `{"duration": 100.0}` {
		t.Errorf("unexpected episode fields: %+v", ep)
	}
	if ep.PubDate != "" || ep.Description != "" || ep.Season != "" || ep.Episode != "" || ep.EpisodeType != "" {
		t.Errorf("expected omitted columns to have default zero values, got: %+v", ep)
	}

	// 6. Test queryPodcastEpisodes with podcast that has no episodes (should return empty slice, no error)
	epsEmpty, err := queryPodcastEpisodes(ctx, db, "podWithNoEpisodes")
	if err != nil {
		t.Fatalf("queryPodcastEpisodes with no episodes failed: %v", err)
	}
	if len(epsEmpty) != 0 {
		t.Errorf("expected 0 episodes, got %d", len(epsEmpty))
	}

	// 7. Test queryPodcastEpisodes with valid podcast ID
	eps, err := queryPodcastEpisodes(ctx, db, "pod1")
	if err != nil {
		t.Fatalf("queryPodcastEpisodes failed: %v", err)
	}
	if len(eps) != 1 {
		t.Fatalf("expected 1 episode, got %d", len(eps))
	}
	ep = eps[0]
	if ep.ID != "ep1" || ep.Title != "Episode 1" {
		t.Errorf("unexpected episode fields in slice: %+v", ep)
	}
}
