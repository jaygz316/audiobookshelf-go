package hls

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "audiobookshelf/internal/logger"
	isocket "audiobookshelf/internal/socket"
)

func parseHLSRequestPath(path string) (streamID, fileName, ext string, err error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	hlsIdx := -1
	for i, part := range parts {
		if part == "hls" {
			hlsIdx = i
			break
		}
	}
	if hlsIdx == -1 || hlsIdx+2 >= len(parts) {
		return "", "", "", fmt.Errorf("hls prefix not found or path too short")
	}
	streamID = parts[hlsIdx+1]
	fileName = parts[hlsIdx+2]

	if filepath.Base(fileName) != fileName {
		return "", "", "", fmt.Errorf("traversal attempt in fileName: %s", fileName)
	}

	ext = filepath.Ext(fileName)
	if ext != ".ts" && ext != ".m3u8" && ext != ".mp4" && ext != ".m4s" {
		return "", "", "", fmt.Errorf("unsupported file format: %s", ext)
	}
	return streamID, fileName, ext, nil
}

func (stream *Stream) waitForSegment(ctx context.Context, fullFilePath string, streamID string, fileName string, ext string, socketAuth *isocket.Authority) bool {
	if _, err := os.Stat(fullFilePath); os.IsNotExist(err) {
		if ext == ".ts" {
			segNum := parseSegmentNumber(fileName)
			if segNum >= 0 {
				sTime, shouldReset := stream.CheckSegmentNumberRequest(segNum)
				if shouldReset {
					log.Printf("[HLS Gateway] Resetting stream %s at segment %d (time %.2fs)", streamID, segNum, sTime)
					_ = stream.Reset(sTime - (stream.SegmentLength * 5.0))
					emitWebsocketEvent(socketAuth, stream.UserID, "stream_reset", map[string]interface{}{
						"startTime": sTime,
						"streamId":  streamID,
					})
				}
			}

			// Wait for the segment to become ready (up to 10 seconds)
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			timeout := time.After(10 * time.Second)
			for {
				select {
				case <-ticker.C:
					if _, err := os.Stat(fullFilePath); err == nil {
						return true
					}
				case <-timeout:
					_, err := os.Stat(fullFilePath)
					return err == nil
				case <-ctx.Done():
					log.Printf("[HLS Gateway] Request context cancelled for %s", fileName)
					return false
				}
			}
		}
	}
	return true
}
