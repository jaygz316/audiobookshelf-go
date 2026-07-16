package metadata

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nwaples/rardecode"
)

// comicInfo XML structures
type comicInfo struct {
	XMLName     xml.Name `xml:"ComicInfo"`
	Title       string   `xml:"Title"`
	Series      string   `xml:"Series"`
	Number      string   `xml:"Number"`
	Writer      string   `xml:"Writer"`
	Publisher   string   `xml:"Publisher"`
	Year        string   `xml:"Year"`
	Month       string   `xml:"Month"`
	Day         string   `xml:"Day"`
	Summary     string   `xml:"Summary"`
	Notes       string   `xml:"Notes"`
	LanguageISO string   `xml:"LanguageISO"`
	ISBN        string   `xml:"ISBN"`
}

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

func extractCbzMetadata(ctx context.Context, filePath string) (*EbookMetadata, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	r, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open cbz reader: %w", err)
	}
	defer r.Close()

	var comicInfoData []byte
	for _, f := range r.File {
		if !f.FileInfo().IsDir() && strings.ToLower(path.Base(f.Name)) == "comicinfo.xml" {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open comicinfo.xml inside cbz: %w", err)
			}
			comicInfoData, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("failed to read comicinfo.xml inside cbz: %w", err)
			}
			break
		}
	}

	var meta *EbookMetadata
	if len(comicInfoData) > 0 {
		var info comicInfo
		if err := xml.Unmarshal(comicInfoData, &info); err == nil {
			meta = mapComicInfoToEbookMetadata(&info)
		}
	}

	if meta == nil {
		meta = &EbookMetadata{}
	}

	if meta.Title == "" {
		meta.Title = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	}

	return meta, nil
}

func extractCbrMetadata(ctx context.Context, filePath string) (*EbookMetadata, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open cbr file: %w", err)
	}
	defer f.Close()

	rr, err := rardecode.NewReader(f, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create rar reader: %w", err)
	}

	var comicInfoData []byte
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		header, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read next rar entry: %w", err)
		}
		if !header.IsDir && strings.ToLower(path.Base(header.Name)) == "comicinfo.xml" {
			comicInfoData, err = io.ReadAll(rr)
			if err != nil {
				return nil, fmt.Errorf("failed to read comicinfo.xml inside cbr: %w", err)
			}
			break
		}
	}

	var meta *EbookMetadata
	if len(comicInfoData) > 0 {
		var info comicInfo
		if err := xml.Unmarshal(comicInfoData, &info); err == nil {
			meta = mapComicInfoToEbookMetadata(&info)
		}
	}

	if meta == nil {
		meta = &EbookMetadata{}
	}

	if meta.Title == "" {
		meta.Title = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	}

	return meta, nil
}

func mapComicInfoToEbookMetadata(info *comicInfo) *EbookMetadata {
	meta := &EbookMetadata{
		Author:      info.Writer,
		Publisher:   info.Publisher,
		Description: info.Summary,
		Language:    info.LanguageISO,
		ISBN:        info.ISBN,
	}

	if info.Series != "" {
		meta.Title = info.Series
		if info.Number != "" {
			meta.Title += " " + info.Number
		}
	} else {
		meta.Title = info.Title
	}

	if info.Year != "" {
		meta.PublishedYear = info.Year
	}

	return meta
}

func extractCbzCover(ctx context.Context, filePath, destPath string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	r, err := zip.OpenReader(filePath)
	if err != nil {
		return fmt.Errorf("failed to open cbz reader: %w", err)
	}
	defer r.Close()

	var imageFiles []string
	for _, f := range r.File {
		if !f.FileInfo().IsDir() && isImageFile(f.Name) {
			if !strings.HasPrefix(filepath.Base(f.Name), ".") && !strings.Contains(f.Name, "__MACOSX") {
				imageFiles = append(imageFiles, f.Name)
			}
		}
	}

	if len(imageFiles) == 0 {
		return errors.New("no images found in cbz archive")
	}

	sort.Slice(imageFiles, func(i, j int) bool {
		return naturalLess(imageFiles[i], imageFiles[j])
	})

	firstImage := imageFiles[0]

	for _, f := range r.File {
		if f.Name == firstImage {
			return extractZipEntry(f, destPath)
		}
	}

	return fmt.Errorf("could not find image file %s in archive", firstImage)
}

func extractCbrCover(ctx context.Context, filePath, destPath string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open cbr file: %w", err)
	}
	defer f.Close()

	rr, err := rardecode.NewReader(f, "")
	if err != nil {
		return fmt.Errorf("failed to create rar reader: %w", err)
	}

	var imageFiles []string
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		header, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read next rar entry: %w", err)
		}
		if !header.IsDir && isImageFile(header.Name) {
			if !strings.HasPrefix(filepath.Base(header.Name), ".") && !strings.Contains(header.Name, "__MACOSX") {
				imageFiles = append(imageFiles, header.Name)
			}
		}
	}

	if len(imageFiles) == 0 {
		return errors.New("no images found in cbr archive")
	}

	sort.Slice(imageFiles, func(i, j int) bool {
		return naturalLess(imageFiles[i], imageFiles[j])
	})

	firstImage := imageFiles[0]

	_, err = f.Seek(0, 0)
	if err != nil {
		return fmt.Errorf("failed to seek cbr file: %w", err)
	}
	rr, err = rardecode.NewReader(f, "")
	if err != nil {
		return fmt.Errorf("failed to recreate rar reader: %w", err)
	}

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		header, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read next rar entry: %w", err)
		}
		if header.Name == firstImage {
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return fmt.Errorf("failed to create destination parent directory: %w", err)
			}
			destFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
			if err != nil {
				return fmt.Errorf("failed to open destination cover file: %w", err)
			}
			_, err = io.Copy(destFile, rr)
			destFile.Close()
			if err != nil {
				return fmt.Errorf("failed to write cover image data: %w", err)
			}
			return nil
		}
	}

	return fmt.Errorf("could not find image file %s in archive", firstImage)
}
