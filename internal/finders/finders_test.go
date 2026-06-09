package finders

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

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

	finder := NewFinder([]providers.Provider{googleMock, itunesMock})

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
		f := NewFinder([]providers.Provider{nil, googleMock})
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

	finder := NewFinder([]providers.Provider{audibleMock, googleMock})

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

		finder := NewFinder([]providers.Provider{p1, p2})

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

		finder := NewFinder([]providers.Provider{p1, p2})

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

func TestSearchAllFailureTolerance(t *testing.T) {
	// Capture log output to assert expected error logging.
	origWriter := log.Writer()
	defer log.SetOutput(origWriter)

	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)

	healthy := &mockProvider{
		name: "healthy",
		searchBooksFn: func(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
			return []*providers.MetadataResult{{Title: "Healthy Book"}}, nil
		},
		searchPodcastsFn: func(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
			return []*providers.MetadataResult{{Title: "Healthy Podcast"}}, nil
		},
	}

	failing := &mockProvider{
		name: "failing",
		searchBooksFn: func(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
			return nil, errors.New("books error")
		},
		searchPodcastsFn: func(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
			return nil, errors.New("podcasts error")
		},
	}

	finder := NewFinder([]providers.Provider{healthy, failing})

	t.Run("SearchBooks all failure tolerance", func(t *testing.T) {
		logBuf.Reset()
		res, err := finder.SearchBooks(context.Background(), "all", "query")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 || res[0].Title != "Healthy Book" {
			t.Errorf("expected only Healthy Book, got %v", res)
		}

		logOutput := logBuf.String()
		expectedLog := "[Finders] provider failing SearchBooks failed: books error"
		if !strings.Contains(logOutput, expectedLog) {
			t.Errorf("expected log to contain %q, got %q", expectedLog, logOutput)
		}
	})

	t.Run("SearchPodcasts all failure tolerance", func(t *testing.T) {
		logBuf.Reset()
		res, err := finder.SearchPodcasts(context.Background(), "", "query")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 || res[0].Title != "Healthy Podcast" {
			t.Errorf("expected only Healthy Podcast, got %v", res)
		}

		logOutput := logBuf.String()
		expectedLog := "[Finders] provider failing SearchPodcasts failed: podcasts error"
		if !strings.Contains(logOutput, expectedLog) {
			t.Errorf("expected log to contain %q, got %q", expectedLog, logOutput)
		}
	})
}

func TestSearchAllContextCancellation(t *testing.T) {
	origWriter := log.Writer()
	defer log.SetOutput(origWriter)

	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)

	p1 := &mockProvider{
		name: "prov1",
		searchBooksFn: func(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
		searchPodcastsFn: func(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}

	finder := NewFinder([]providers.Provider{p1})

	t.Run("SearchBooks context cancellation", func(t *testing.T) {
		logBuf.Reset()
		ctx, cancel := context.WithCancel(context.Background())

		errChan := make(chan error, 1)
		go func() {
			_, err := finder.SearchBooks(ctx, "all", "query")
			errChan <- err
		}()

		cancel()

		select {
		case err := <-errChan:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("expected context.Canceled error, got %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for cancelled search to return")
		}

		if logBuf.Len() > 0 {
			t.Errorf("expected no error logs on context cancellation, got: %s", logBuf.String())
		}
	})

	t.Run("SearchPodcasts context cancellation", func(t *testing.T) {
		logBuf.Reset()
		ctx, cancel := context.WithCancel(context.Background())

		errChan := make(chan error, 1)
		go func() {
			_, err := finder.SearchPodcasts(ctx, "", "query")
			errChan <- err
		}()

		cancel()

		select {
		case err := <-errChan:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("expected context.Canceled error, got %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for cancelled search to return")
		}

		if logBuf.Len() > 0 {
			t.Errorf("expected no error logs on context cancellation, got: %s", logBuf.String())
		}
	})
}
