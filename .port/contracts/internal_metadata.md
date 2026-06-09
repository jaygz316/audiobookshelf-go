# Package internal/metadata

This package handles parsing EPUB and comic book archives (CBZ/CBR/PDF) to extract core metadata, chapters, and cover images.

## Go Signatures

```go
package metadata

import (
	"context"
)

type EbookMetadata struct {
	Title         string    `json:"title"`
	Author        string    `json:"author"`
	Publisher     string    `json:"publisher"`
	PublishedYear string    `json:"publishedYear"`
	Description   string    `json:"description"`
	Language      string    `json:"language"`
	ISBN          string    `json:"isbn"`
	Chapters      []Chapter `json:"chapters"`
}

type Chapter struct {
	ID          int     `json:"id"`
	Title       string  `json:"title"`
	StartOffset float64 `json:"startOffset"` // in seconds
	EndOffset   float64 `json:"endOffset"`   // in seconds
}

// ExtractEpubMetadata parses a local EPUB file and extracts its metadata and table of contents.
func ExtractEpubMetadata(ctx context.Context, filePath string) (*EbookMetadata, error)

// ExtractEpubCover parses an EPUB file, locates the cover page/image, and writes it to the destination cover file.
func ExtractEpubCover(ctx context.Context, filePath, destPath string) error

// ExtractComicMetadata parses Comic metadata (ComicInfo.xml, etc.) and cover image from CBZ, CBR, or PDF.
func ExtractComicMetadata(ctx context.Context, filePath string) (*EbookMetadata, error)
```

## Behavioral Notes
- **ExtractEpubMetadata**: Reads internal OPF files inside the EPUB zip container and decodes standard dc/opf metadata fields.
- **ExtractEpubCover**: Searches EPUB manifest for properties containing "cover-image" or files matching cover naming patterns, extracting the file to disk.
- **ExtractComicMetadata**: Checks for `ComicInfo.xml` inside CBZ/CBR file to parse tags and uses a helper (like `pdfinfo` or zip reader) to extract the first page as a cover image.
