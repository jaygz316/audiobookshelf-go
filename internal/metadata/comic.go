package metadata

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// ExtractComicMetadata parses Comic metadata (ComicInfo.xml, etc.) and cover image from CBZ, CBR, or PDF.
func ExtractComicMetadata(ctx context.Context, filePath string) (*EbookMetadata, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".cbz":
		return extractCbzMetadata(ctx, filePath)
	case ".cbr":
		return extractCbrMetadata(ctx, filePath)
	case ".pdf":
		return parsePdfInfo(ctx, filePath)
	default:
		return nil, fmt.Errorf("unsupported comic format: %s", ext)
	}
}

// ExtractComicCover extracts the cover page/image from CBZ, CBR, or PDF and writes it to destPath.
func ExtractComicCover(ctx context.Context, filePath, destPath string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".cbz":
		return extractCbzCover(ctx, filePath, destPath)
	case ".cbr":
		return extractCbrCover(ctx, filePath, destPath)
	case ".pdf":
		return extractPdfCover(ctx, filePath, destPath)
	default:
		return fmt.Errorf("unsupported comic format: %s", ext)
	}
}
