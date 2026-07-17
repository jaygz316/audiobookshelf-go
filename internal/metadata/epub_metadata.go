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
	"strings"
)

// ExtractEpubMetadata parses a local EPUB file and extracts its metadata and table of contents.
func ExtractEpubMetadata(ctx context.Context, filePath string) (*EbookMetadata, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	r, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open zip reader: %w", err)
	}
	defer r.Close()

	// Locate container.xml
	containerFile := findZipEntryCaseInsensitive(&r.Reader, "META-INF/container.xml")
	if containerFile == nil {
		return nil, errors.New("META-INF/container.xml not found")
	}

	// Read container.xml
	rc, err := containerFile.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open container.xml: %w", err)
	}
	defer rc.Close()

	containerData, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("failed to read container.xml: %w", err)
	}

	var container container
	if err := xml.Unmarshal(containerData, &container); err != nil {
		return nil, fmt.Errorf("failed to unmarshal container.xml: %w", err)
	}

	if len(container.Rootfiles.Rootfile) == 0 {
		return nil, errors.New("no rootfile found in container.xml")
	}

	packageDocPath := container.Rootfiles.Rootfile[0].FullPath
	opfFile := findZipEntryCaseInsensitive(&r.Reader, packageDocPath)
	if opfFile == nil {
		return nil, fmt.Errorf("opf file not found at %s", packageDocPath)
	}

	// Read OPF file
	rcOPF, err := opfFile.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open opf file: %w", err)
	}
	defer rcOPF.Close()

	opfData, err := io.ReadAll(rcOPF)
	if err != nil {
		return nil, fmt.Errorf("failed to read opf file: %w", err)
	}

	var opf opfPackage
	if err := xml.Unmarshal(opfData, &opf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal opf file: %w", err)
	}

	meta := mapOpfToEbookMetadata(&opf)

	// Resolve chapters
	tocHref, tocType := findTocPath(&opf)
	if tocHref != "" {
		tocZipPath := path.Clean(path.Join(path.Dir(packageDocPath), tocHref))
		tocFile := findZipEntryCaseInsensitive(&r.Reader, tocZipPath)
		if tocFile != nil {
			rcTOC, err := tocFile.Open()
			if err == nil {
				defer rcTOC.Close()
				tocData, err := io.ReadAll(rcTOC)
				if err == nil {
					if tocType == "ncx" {
						var ncx ncx
						if err := xml.Unmarshal(tocData, &ncx); err == nil {
							var chapters []Chapter
							flattenNcxNavPoints(ncx.NavMap.NavPoints, &chapters)
							meta.Chapters = chapters
						}
					} else if tocType == "nav" {
						if chapters, err := parseNavChapters(tocData); err == nil {
							meta.Chapters = chapters
						}
					}
				}
			}
		}
	}

	if meta.Chapters == nil {
		meta.Chapters = []Chapter{}
	}

	if meta.Title == "" {
		meta.Title = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	}

	return meta, nil
}
