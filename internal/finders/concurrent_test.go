package finders

import (
	"context"
	"testing"
	"time"

	"audiobookshelf/internal/providers"
)

func TestConcurrentSearch(t *testing.T) {
	t.Run("SearchBooks with 'all' runs concurrently", func(t *testing.T) {
		startSig := make(chan struct{})
		entered := make(chan struct{}, 2)

		p1 := &mockProvider{
			name: "prov1",
			searchBooksFn: func(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
				entered <- struct{}{}
				select {
				case <-startSig:
					return []*providers.MetadataResult{{Title: "Book 1"}}, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			},
		}

		p2 := &mockProvider{
			name: "prov2",
			searchBooksFn: func(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
				entered <- struct{}{}
				select {
				case <-startSig:
					return []*providers.MetadataResult{{Title: "Book 2"}}, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			},
		}

		finder := NewFinder(nil, []providers.Provider{p1, p2})

		type searchResult struct {
			res []*providers.MetadataResult
			err error
		}

		resChan := make(chan searchResult, 1)
		go func() {
			res, err := finder.SearchBooks(context.Background(), "all", "query")
			resChan <- searchResult{res, err}
		}()

		// Wait until both providers have entered SearchBooks before releasing the block.
		<-entered
		<-entered
		close(startSig)

		select {
		case result := <-resChan:
			if result.err != nil {
				t.Fatalf("unexpected error: %v", result.err)
			}
			if len(result.res) != 2 {
				t.Fatalf("expected 2 results, got %d", len(result.res))
			}
			titles := map[string]bool{}
			for _, r := range result.res {
				titles[r.Title] = true
			}
			if !titles["Book 1"] || !titles["Book 2"] {
				t.Errorf("expected both Book 1 and Book 2, got %v", titles)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for concurrent search to complete")
		}
	})

	t.Run("SearchPodcasts with empty provider runs concurrently", func(t *testing.T) {
		startSig := make(chan struct{})
		entered := make(chan struct{}, 2)

		p1 := &mockProvider{
			name: "prov1",
			searchPodcastsFn: func(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
				entered <- struct{}{}
				select {
				case <-startSig:
					return []*providers.MetadataResult{{Title: "Podcast 1"}}, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			},
		}

		p2 := &mockProvider{
			name: "prov2",
			searchPodcastsFn: func(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
				entered <- struct{}{}
				select {
				case <-startSig:
					return []*providers.MetadataResult{{Title: "Podcast 2"}}, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			},
		}

		finder := NewFinder(nil, []providers.Provider{p1, p2})

		type searchResult struct {
			res []*providers.MetadataResult
			err error
		}

		resChan := make(chan searchResult, 1)
		go func() {
			res, err := finder.SearchPodcasts(context.Background(), "", "query")
			resChan <- searchResult{res, err}
		}()

		// Wait until both providers have entered SearchPodcasts before releasing the block.
		<-entered
		<-entered
		close(startSig)

		select {
		case result := <-resChan:
			if result.err != nil {
				t.Fatalf("unexpected error: %v", result.err)
			}
			if len(result.res) != 2 {
				t.Fatalf("expected 2 results, got %d", len(result.res))
			}
			titles := map[string]bool{}
			for _, r := range result.res {
				titles[r.Title] = true
			}
			if !titles["Podcast 1"] || !titles["Podcast 2"] {
				t.Errorf("expected both Podcast 1 and Podcast 2, got %v", titles)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for concurrent search to complete")
		}
	})
}
