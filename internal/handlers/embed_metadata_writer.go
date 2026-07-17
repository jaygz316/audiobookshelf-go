package handlers

import (
	"bytes"
	"fmt"
	"os"
	"strings"
)

func writeFFMetadataFile(meta *bookEmbedMetadata) (string, error) {
	var metadataBuf bytes.Buffer
	metadataBuf.WriteString(";FFMETADATA1\n")
	metadataBuf.WriteString(fmt.Sprintf("title=%s\n", escapeMetadataValue(meta.Title)))
	metadataBuf.WriteString(fmt.Sprintf("artist=%s\n", escapeMetadataValue(meta.AuthorName)))
	metadataBuf.WriteString(fmt.Sprintf("album=%s\n", escapeMetadataValue(meta.Title)))
	metadataBuf.WriteString(fmt.Sprintf("album_artist=%s\n", escapeMetadataValue(meta.AuthorName)))

	if len(meta.Narrators) > 0 {
		metadataBuf.WriteString(fmt.Sprintf("composer=%s\n", escapeMetadataValue(strings.Join(meta.Narrators, ", "))))
	}
	if meta.Description != "" {
		metadataBuf.WriteString(fmt.Sprintf("description=%s\n", escapeMetadataValue(meta.Description)))
		metadataBuf.WriteString(fmt.Sprintf("comment=%s\n", escapeMetadataValue(meta.Description)))
	}
	if meta.Publisher != "" {
		metadataBuf.WriteString(fmt.Sprintf("publisher=%s\n", escapeMetadataValue(meta.Publisher)))
	}
	dateStr := meta.PublishedYear
	if meta.PublishedDate != "" {
		dateStr = meta.PublishedDate
	}
	if dateStr != "" {
		metadataBuf.WriteString(fmt.Sprintf("date=%s\n", escapeMetadataValue(dateStr)))
	}
	allGenresAndTags := append(meta.Genres, meta.Tags...)
	if len(allGenresAndTags) > 0 {
		metadataBuf.WriteString(fmt.Sprintf("genre=%s\n", escapeMetadataValue(strings.Join(allGenresAndTags, ", "))))
	}

	// Write chapters
	for _, c := range meta.Chapters {
		metadataBuf.WriteString("\n[CHAPTER]\n")
		metadataBuf.WriteString("TIMEBASE=1/1000\n")
		metadataBuf.WriteString(fmt.Sprintf("START=%d\n", int64(c.Start*1000)))
		metadataBuf.WriteString(fmt.Sprintf("END=%d\n", int64(c.End*1000)))
		metadataBuf.WriteString(fmt.Sprintf("title=%s\n", escapeMetadataValue(c.Title)))
	}

	// Create a temporary FFMETADATA file
	metaFile, err := os.CreateTemp("", "ffmetadata-*.txt")
	if err != nil {
		return "", fmt.Errorf("Failed to create temp metadata file: %w", err)
	}

	if _, err := metaFile.Write(metadataBuf.Bytes()); err != nil {
		metaFile.Close()
		os.Remove(metaFile.Name())
		return "", fmt.Errorf("Failed to write temp metadata: %w", err)
	}
	metaFile.Close()
	return metaFile.Name(), nil
}

func escapeMetadataValue(val string) string {
	val = strings.ReplaceAll(val, "\\", "\\\\")
	val = strings.ReplaceAll(val, "=", "\\=")
	val = strings.ReplaceAll(val, ";", "\\;")
	val = strings.ReplaceAll(val, "#", "\\#")
	val = strings.ReplaceAll(val, "\n", "\\\n")
	return val
}
