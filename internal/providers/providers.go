package providers

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/doyensec/safeurl"
)

// MetadataResult standardized metadata info.
type MetadataResult struct {
	Title         string   `json:"title"`
	Subtitle      string   `json:"subtitle"`
	Authors       []string `json:"authors"`
	Narrators     []string `json:"narrators"`
	Publisher     string   `json:"publisher"`
	PublishedYear string   `json:"publishedYear"`
	Description   string   `json:"description"`
	Language      string   `json:"language"`
	ISBN          string   `json:"isbn"`
	ASIN          string   `json:"asin"`
	CoverURL      string   `json:"coverUrl"`
	FeedURL       string   `json:"feedUrl,omitempty"`
}

// Provider external metadata source client.
type Provider interface {
	Name() string
	SearchBooks(ctx context.Context, query string) ([]*MetadataResult, error)
	SearchPodcasts(ctx context.Context, query string) ([]*MetadataResult, error)
}

var safeHTTPClient *safeurl.WrappedClient
var asinRegex = regexp.MustCompile(`^[A-Z0-9]{10}$`)
var htmlTagRegex = regexp.MustCompile(`<[^>]*>`)

func init() {
	config := safeurl.GetConfigBuilder().Build()
	safeHTTPClient = safeurl.Client(config)
}

func isValidASIN(str string) bool {
	return asinRegex.MatchString(strings.ToUpper(str))
}

func cleanDescription(desc string) string {
	stripped := htmlTagRegex.ReplaceAllString(desc, "")
	return html.UnescapeString(stripped)
}

// toTitle converts a string to title case, mimicking strings.Title
// but unicode-safe and avoiding the deprecation warning.
func toTitle(s string) string {
	if s == "" {
		return ""
	}
	var build strings.Builder
	nextUpper := true
	for _, r := range s {
		if unicode.IsSpace(r) {
			nextUpper = true
			build.WriteRune(r)
		} else if nextUpper {
			build.WriteRune(unicode.ToUpper(r))
			nextUpper = false
		} else {
			build.WriteRune(r)
		}
	}
	return build.String()
}

// getWithRetry retries GET requests if status is 429 Rate Limited.
func getWithRetry(ctx context.Context, client *safeurl.WrappedClient, urlStr string) (*http.Response, error) {
	for {
		req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("HTTP request failed: %w", err)
		}

		if resp.StatusCode == 429 {
			retryAfter := 5
			if h := resp.Header.Get("Retry-After"); h != "" {
				if sec, err := strconv.Atoi(h); err == nil {
					retryAfter = sec
				}
			}
			resp.Body.Close()

			timer := time.NewTimer(time.Duration(retryAfter) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
				continue
			}
		}

		return resp, nil
	}
}

// DownloadURL downloads a URL using the safe HTTP client.
func DownloadURL(ctx context.Context, urlStr string) ([]byte, error) {
	resp, err := getWithRetry(ctx, safeHTTPClient, urlStr)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
	if err != nil {
		return nil, err
	}
	return data, nil
}

// SetSafeHTTPClientForTest overrides the global safe HTTP client for testing.
func SetSafeHTTPClientForTest(client *safeurl.WrappedClient) {
	safeHTTPClient = client
}
