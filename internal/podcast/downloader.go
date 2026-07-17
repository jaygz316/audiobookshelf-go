package podcast

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// DownloadEpisode streams an episode's audio media enclosure to a local file path.
func (m *PodcastManager) DownloadEpisode(ctx context.Context, episodeURL, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", episodeURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", getUserAgent(episodeURL))

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create destination file: %w", err)
	}
	defer func() {
		_ = out.Close()
	}()

	// PORT: Chunked streaming using io.Copy for streaming efficiency.
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("stream copy failed: %w", err)
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("close destination file: %w", err)
	}

	return nil
}

// DownloadEpisodeWithProgress downloads a podcast episode with progress callback.
func (m *PodcastManager) DownloadEpisodeWithProgress(ctx context.Context, episodeURL, destPath string, onProgress func(bytesDownloaded, bytesTotal int64)) error {
	req, err := http.NewRequestWithContext(ctx, "GET", episodeURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", getUserAgent(episodeURL))

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	totalBytes := resp.ContentLength

	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create destination file: %w", err)
	}
	defer func() {
		_ = out.Close()
	}()

	buffer := make([]byte, 32*1024)
	var downloaded int64
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			_, writeErr := out.Write(buffer[:n])
			if writeErr != nil {
				return fmt.Errorf("write error: %w", writeErr)
			}
			downloaded += int64(n)
			if onProgress != nil {
				onProgress(downloaded, totalBytes)
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return fmt.Errorf("read error: %w", readErr)
		}
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("close destination file: %w", err)
	}

	return nil
}

func getUserAgent(urlStr string) string {
	userAgent := "audiobookshelf (+https://audiobookshelf.org; like iTMS)"
	if strings.HasPrefix(urlStr, "https://www.cbc.ca") {
		userAgent = "audiobookshelf (+https://audiobookshelf.org; like iTMS) - CBC"
	}
	return userAgent
}
