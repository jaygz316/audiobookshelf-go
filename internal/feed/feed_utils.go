package feed

import (
	"crypto/md5"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"
)

// Utility and Helper Functions
func checkUseChapterTitles(tracks []audiobookTrack, chapters []audiobookChapter) bool {
	if len(tracks) != len(chapters) {
		return false
	}
	for i := 0; i < len(tracks); i++ {
		if math.Abs(chapters[i].Start-tracks[i].StartOffset) >= 1.0 {
			return false
		}
	}
	return true
}

func parseTime(s string) time.Time {
	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(ms/1000, (ms%1000)*1000000)
	}
	return time.Time{}
}

func sortPodcastEpisodes(eps []*podcastEpData, descending bool) {
	sort.Slice(eps, func(i, j int) bool {
		tI := parseTime(eps[i].PubDate)
		tJ := parseTime(eps[j].PubDate)
		if descending {
			return tI.After(tJ)
		}
		return tI.Before(tJ)
	})
}

func formatPubDate(dateStr string) string {
	if dateStr == "" {
		return time.Now().UTC().Format(time.RFC1123Z)
	}
	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, dateStr); err == nil {
			return t.UTC().Format(time.RFC1123Z)
		}
	}
	if ms, err := strconv.ParseInt(dateStr, 10, 64); err == nil {
		return time.Unix(ms/1000, (ms%1000)*1000000).UTC().Format(time.RFC1123Z)
	}
	return dateStr
}

func computeMD5(val string) string {
	h := md5.New()
	h.Write([]byte(val))
	return fmt.Sprintf("%x", h.Sum(nil))
}
