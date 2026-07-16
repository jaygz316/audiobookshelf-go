package hls

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	log "audiobookshelf/internal/logger"
	isocket "audiobookshelf/internal/socket"
)

var segNumRegex = regexp.MustCompile(`output-(\d+)\.ts`)

func escapeSingleQuotes(path string) string {
	p := filepath.ToSlash(path)
	return strings.ReplaceAll(p, "'", "'\\''")
}

func parseSegmentNumber(name string) int {
	matches := segNumRegex.FindStringSubmatch(name)
	if len(matches) < 2 {
		return -1
	}
	num, err := strconv.Atoi(matches[1])
	if err != nil {
		return -1
	}
	return num
}

// GetPlaylistStr builds an HLS playlist string (exported for testing).
func GetPlaylistStr(segmentName string, duration float64, segmentLength float64, hlsSegmentType string) string {
	return getPlaylistStr(segmentName, duration, segmentLength, hlsSegmentType)
}

func getPlaylistStr(segmentName string, duration float64, segmentLength float64, hlsSegmentType string) string {
	ext := "ts"
	if hlsSegmentType == "fmp4" {
		ext = "m4s"
	}
	lines := []string{
		"#EXTM3U",
		"#EXT-X-VERSION:3",
		"#EXT-X-ALLOW-CACHE:NO",
		"#EXT-X-TARGETDURATION:" + fmt.Sprintf("%.0f", segmentLength),
		"#EXT-X-MEDIA-SEQUENCE:0",
		"#EXT-X-PLAYLIST-TYPE:VOD",
	}
	if hlsSegmentType == "fmp4" {
		lines = append(lines, `#EXT-X-MAP:URI="init.mp4"`)
	}
	numSegments := int(duration / segmentLength)
	lastSegment := duration - float64(numSegments)*segmentLength
	for i := 0; i < numSegments; i++ {
		lines = append(lines, fmt.Sprintf("#EXTINF:%.0f,", segmentLength))
		lines = append(lines, fmt.Sprintf("%s-%d.%s", segmentName, i, ext))
	}
	if lastSegment > 0 {
		lines = append(lines, fmt.Sprintf("#EXTINF:%g,", lastSegment))
		lines = append(lines, fmt.Sprintf("%s-%d.%s", segmentName, numSegments, ext))
	}
	lines = append(lines, "#EXT-X-ENDLIST")
	return strings.Join(lines, "\n")
}

func emitWebsocketEvent(socketAuth *isocket.Authority, userID string, event string, payload interface{}) {
	if socketAuth != nil {
		socketAuth.ClientEmitter(userID, event, payload)
	} else {
		log.Printf("[HLS Stream Warning] SocketAuth not initialized, cannot emit: %s", event)
	}
}
