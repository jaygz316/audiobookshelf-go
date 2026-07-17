package metadata

import (
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
