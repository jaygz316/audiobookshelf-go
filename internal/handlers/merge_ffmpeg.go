package handlers

import (
	log "audiobookshelf/internal/logger"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func escapeConcatPath(path string) string {
	return strings.ReplaceAll(path, "'", "'\\''")
}

// runMergeFFmpeg sets up metadata/concat files and runs the ffmpeg subprocess.
func runMergeFFmpeg(ctx context.Context, mergeCtx *MergeContext) ([]MergeChapter, float64, int, error) {
	concatFile, err := os.CreateTemp("", "ffmpeg-concat-*.txt")
	if err != nil {
		return nil, 0, http.StatusInternalServerError, fmt.Errorf("Failed to create temporary concat file: %v", err)
	}
	defer os.Remove(concatFile.Name())
	defer concatFile.Close()

	for _, af := range mergeCtx.ActiveFiles {
		_, _ = concatFile.WriteString(fmt.Sprintf("file '%s'\n", escapeConcatPath(af.Metadata.Path)))
	}
	_ = concatFile.Close()

	var chapters []MergeChapter
	var currentOffset float64 = 0.0
	for i, af := range mergeCtx.ActiveFiles {
		chTitle := af.Title
		if chTitle == "" {
			filename := af.Metadata.Filename
			ext := filepath.Ext(filename)
			chTitle = strings.TrimSuffix(filename, ext)
		}

		chapters = append(chapters, MergeChapter{
			ID:    i + 1,
			Start: currentOffset,
			End:   currentOffset + af.Duration,
			Title: chTitle,
		})
		currentOffset += af.Duration
	}

	metadataBuf := bytes.Buffer{}
	metadataBuf.WriteString(";FFMETADATA1\n")
	metadataBuf.WriteString(fmt.Sprintf("title=%s\n", escapeMetadataValue(mergeCtx.Title)))
	authorNameStr := ""
	if mergeCtx.AuthorName.Valid {
		authorNameStr = mergeCtx.AuthorName.String
	}
	metadataBuf.WriteString(fmt.Sprintf("artist=%s\n", escapeMetadataValue(authorNameStr)))
	metadataBuf.WriteString(fmt.Sprintf("album=%s\n", escapeMetadataValue(mergeCtx.Title)))

	for _, c := range chapters {
		metadataBuf.WriteString("\n[CHAPTER]\n")
		metadataBuf.WriteString("TIMEBASE=1/1000\n")
		metadataBuf.WriteString(fmt.Sprintf("START=%d\n", int64(c.Start*1000)))
		metadataBuf.WriteString(fmt.Sprintf("END=%d\n", int64(c.End*1000)))
		metadataBuf.WriteString(fmt.Sprintf("title=%s\n", escapeMetadataValue(c.Title)))
	}

	metadataFile, err := os.CreateTemp("", "ffmpeg-metadata-*.txt")
	if err != nil {
		return nil, 0, http.StatusInternalServerError, fmt.Errorf("Failed to create temporary metadata file: %v", err)
	}
	defer os.Remove(metadataFile.Name())
	defer metadataFile.Close()

	_, _ = metadataFile.Write(metadataBuf.Bytes())
	_ = metadataFile.Close()

	args := []string{
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", concatFile.Name(),
		"-i", metadataFile.Name(),
		"-map_metadata", "1",
		"-map", "0:a",
	}

	if mergeCtx.UseCopy {
		args = append(args, "-c", "copy")
	} else {
		args = append(args, "-c:a", "aac", "-b:a", "128k")
	}
	args = append(args, mergeCtx.OutputPath)

	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	log.Infof("[Go] Running ffmpeg command: ffmpeg %s", strings.Join(args, " "))
	if err := cmd.Run(); err != nil {
		log.Errorf("[Error] FFmpeg merge execution failed: %v, stderr: %s", err, stderr.String())
		os.Remove(mergeCtx.OutputPath)
		return nil, 0, http.StatusInternalServerError, fmt.Errorf("FFmpeg merge execution failed: %v. Stderr: %s", err, stderr.String())
	}

	mergedStat, err := os.Stat(mergeCtx.OutputPath)
	if err != nil || mergedStat.Size() < 1024 {
		log.Errorf("[Error] Merged file missing or too small")
		os.Remove(mergeCtx.OutputPath)
		return nil, 0, http.StatusInternalServerError, fmt.Errorf("Merged file missing or too small")
	}

	return chapters, currentOffset, 0, nil
}
