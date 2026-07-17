package finders

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	log "audiobookshelf/internal/logger"
	"audiobookshelf/internal/providers"
)

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

	finder := NewFinder(nil, []providers.Provider{healthy, failing})

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

	finder := NewFinder(nil, []providers.Provider{p1})

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
