package metadata

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

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
