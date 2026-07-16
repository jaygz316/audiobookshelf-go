package metadata

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func parsePdfInfo(ctx context.Context, filePath string) (*EbookMetadata, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	cmd := exec.CommandContext(ctx, "pdfinfo", filePath)
	out, err := cmd.Output()
	if err != nil {
		return &EbookMetadata{
			Title: strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath)),
		}, nil
	}

	meta := &EbookMetadata{}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "Title":
			meta.Title = val
		case "Author":
			meta.Author = val
		case "Publisher":
			meta.Publisher = val
		case "CreationDate":
			meta.PublishedYear = extractYearFromPdfDate(val)
		}
	}

	if meta.Title == "" {
		meta.Title = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	}
	return meta, nil
}

func extractYearFromPdfDate(dateStr string) string {
	if match := yearRegex.FindString(dateStr); match != "" {
		return match
	}
	if strings.HasPrefix(dateStr, "D:") && len(dateStr) >= 6 {
		return dateStr[2:6]
	}
	return ""
}

func extractPdfCover(ctx context.Context, filePath, destPath string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	tmpDir, err := os.MkdirTemp("", "pdf-cover-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	outputPrefix := filepath.Join(tmpDir, "cover")
	cmd := exec.CommandContext(ctx, "pdftoppm", "-jpeg", "-f", "1", "-l", "1", "-singlefile", filePath, outputPrefix)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pdftoppm failed: %w", err)
	}

	resultPath := outputPrefix + ".jpg"
	return copyFile(resultPath, destPath)
}
