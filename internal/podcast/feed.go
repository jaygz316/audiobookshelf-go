package podcast

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// FetchFeed parses a remote RSS feed URL.
func (m *PodcastManager) FetchFeed(ctx context.Context, url string) (*PodcastFeed, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", getUserAgent(url))
	req.Header.Set("Accept", "application/rss+xml, application/xhtml+xml, application/xml, */*;q=0.8")
	req.Header.Set("Accept-Encoding", "gzip, compress, deflate")

	resp, err := m.client.Do(req)
	if err != nil {
		// PORT: Redirect fallback from http to https in case of protocol/redirection error.
		if strings.HasPrefix(url, "http://") {
			upgradedURL := strings.Replace(url, "http://", "https://", 1)
			reqUpgraded, errUpgraded := http.NewRequestWithContext(ctx, "GET", upgradedURL, nil)
			if errUpgraded == nil {
				reqUpgraded.Header.Set("User-Agent", getUserAgent(upgradedURL))
				reqUpgraded.Header.Set("Accept", "application/rss+xml, application/xhtml+xml, application/xml, */*;q=0.8")
				reqUpgraded.Header.Set("Accept-Encoding", "gzip, compress, deflate")
				respUpgraded, errUpgradedCall := m.client.Do(reqUpgraded)
				if errUpgradedCall == nil {
					if resp != nil && resp.Body != nil {
						resp.Body.Close()
					}
					resp = respUpgraded
					err = nil
				}
			}
		}
	}

	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	// PORT: Support iso-8859-1 and windows-1252 encoded feeds by converting Latin1 string to UTF-8
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "iso-8859-1") || strings.Contains(contentType, "windows-1252") {
		bodyBytes = []byte(latin1ToUTF8(string(bodyBytes)))

		limit := 500
		if len(bodyBytes) < limit {
			limit = len(bodyBytes)
		}
		prefix := string(bodyBytes[:limit])
		re := regexp.MustCompile(`(?i)encoding=["'](?:iso-8859-1|windows-1252)["']`)
		prefix = re.ReplaceAllString(prefix, `encoding="utf-8"`)
		bodyBytes = append([]byte(prefix), bodyBytes[limit:]...)
	}

	feed, err := parseRSS(bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse RSS XML: %w", err)
	}

	return feed, nil
}
