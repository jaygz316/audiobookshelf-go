package finders

import (
	"context"
	"sync"
	"testing"

	"audiobookshelf/internal/providers"
)

// mockProvider implements providers.Provider for testing.
type mockProvider struct {
	name             string
	searchBooksFn    func(ctx context.Context, query string) ([]*providers.MetadataResult, error)
	searchPodcastsFn func(ctx context.Context, query string) ([]*providers.MetadataResult, error)
	booksCalls       int
	podcastsCalls    int
	mu               sync.Mutex
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) SearchBooks(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
	m.mu.Lock()
	m.booksCalls++
	m.mu.Unlock()
	if m.searchBooksFn != nil {
		return m.searchBooksFn(ctx, query)
	}
	return nil, nil
}

func (m *mockProvider) SearchPodcasts(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
	m.mu.Lock()
	m.podcastsCalls++
	m.mu.Unlock()
	if m.searchPodcastsFn != nil {
		return m.searchPodcastsFn(ctx, query)
	}
	return nil, nil
}

func (m *mockProvider) getCalls() (books, podcasts int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.booksCalls, m.podcastsCalls
}

func TestSearchRouting(t *testing.T) {
	googleMock := &mockProvider{
		name: "google",
		searchBooksFn: func(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
			return []*providers.MetadataResult{{Title: "Google Book"}}, nil
		},
		searchPodcastsFn: func(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
			return []*providers.MetadataResult{{Title: "Google Podcast"}}, nil
		},
	}

	itunesMock := &mockProvider{
		name: "itunes",
		searchBooksFn: func(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
			return []*providers.MetadataResult{{Title: "iTunes Book"}}, nil
		},
		searchPodcastsFn: func(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
			return []*providers.MetadataResult{{Title: "iTunes Podcast"}}, nil
		},
	}

	finder := NewFinder(nil, []providers.Provider{googleMock, itunesMock})

	t.Run("Route SearchBooks to google", func(t *testing.T) {
		res, err := finder.SearchBooks(context.Background(), "google", "query")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 || res[0].Title != "Google Book" {
			t.Errorf("expected Google Book, got %v", res)
		}
		bCalls, pCalls := googleMock.getCalls()
		if bCalls != 1 || pCalls != 0 {
			t.Errorf("expected google mock called 1 time for books, got books=%d podcasts=%d", bCalls, pCalls)
		}
	})

	t.Run("Route SearchPodcasts to itunes", func(t *testing.T) {
		res, err := finder.SearchPodcasts(context.Background(), "itunes", "query")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 || res[0].Title != "iTunes Podcast" {
			t.Errorf("expected iTunes Podcast, got %v", res)
		}
		bCalls, pCalls := itunesMock.getCalls()
		if bCalls != 0 || pCalls != 1 {
			t.Errorf("expected itunes mock called 1 time for podcasts, got books=%d podcasts=%d", bCalls, pCalls)
		}
	})

	t.Run("Route to non-existent provider returns error", func(t *testing.T) {
		_, err := finder.SearchBooks(context.Background(), "unknown", "query")
		if err == nil {
			t.Error("expected error for unknown provider, got nil")
		}
		expectedErr := `provider "unknown" not found`
		if err != nil && err.Error() != expectedErr {
			t.Errorf("expected error %q, got %q", expectedErr, err.Error())
		}
	})

	t.Run("Ignore nil provider in initialization", func(t *testing.T) {
		f := NewFinder(nil, []providers.Provider{nil, googleMock})
		if len(f.providers) != 1 {
			t.Errorf("expected 1 provider, got %d", len(f.providers))
		}
	})
}

func TestCaseInsensitivityAndRegionFallback(t *testing.T) {
	audibleMock := &mockProvider{
		name: "audible",
		searchBooksFn: func(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
			return []*providers.MetadataResult{{Title: "Audible Book"}}, nil
		},
		searchPodcastsFn: func(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
			return []*providers.MetadataResult{{Title: "Audible Podcast"}}, nil
		},
	}

	googleMock := &mockProvider{
		name: "google",
		searchBooksFn: func(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
			return []*providers.MetadataResult{{Title: "Google Book"}}, nil
		},
	}

	finder := NewFinder(nil, []providers.Provider{audibleMock, googleMock})

	t.Run("Case-insensitive lookup", func(t *testing.T) {
		res, err := finder.SearchBooks(context.Background(), "GoOgLe", "query")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 || res[0].Title != "Google Book" {
			t.Errorf("expected Google Book, got %v", res)
		}
	})

	t.Run("Audible region fallback audible.ca", func(t *testing.T) {
		res, err := finder.SearchBooks(context.Background(), "audible.ca", "query")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 || res[0].Title != "Audible Book" {
			t.Errorf("expected Audible Book, got %v", res)
		}
	})

	t.Run("Audible region fallback case-insensitive AUDIBLE.CO.UK", func(t *testing.T) {
		res, err := finder.SearchPodcasts(context.Background(), "AUDIBLE.CO.UK", "query")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 || res[0].Title != "Audible Podcast" {
			t.Errorf("expected Audible Podcast, got %v", res)
		}
	})
}
