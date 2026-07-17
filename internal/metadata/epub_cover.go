package metadata

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"path"
)

// ExtractEpubCover parses an EPUB file, locates the cover page/image, and writes it to the destination cover file.
func ExtractEpubCover(ctx context.Context, filePath, destPath string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	r, err := zip.OpenReader(filePath)
	if err != nil {
		return fmt.Errorf("failed to open zip reader: %w", err)
	}
	defer r.Close()

	containerFile := findZipEntryCaseInsensitive(&r.Reader, "META-INF/container.xml")
	if containerFile == nil {
		return extractCoverByNameFallback(&r.Reader, destPath)
	}

	rc, err := containerFile.Open()
	if err != nil {
		return fmt.Errorf("failed to open container.xml: %w", err)
	}
	defer rc.Close()

	containerData, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("failed to read container.xml: %w", err)
	}

	var container container
	if err := xml.Unmarshal(containerData, &container); err != nil {
		return fmt.Errorf("failed to unmarshal container.xml: %w", err)
	}

	if len(container.Rootfiles.Rootfile) == 0 {
		return extractCoverByNameFallback(&r.Reader, destPath)
	}

	packageDocPath := container.Rootfiles.Rootfile[0].FullPath
	opfFile := findZipEntryCaseInsensitive(&r.Reader, packageDocPath)
	if opfFile == nil {
		return extractCoverByNameFallback(&r.Reader, destPath)
	}

	rcOPF, err := opfFile.Open()
	if err != nil {
		return fmt.Errorf("failed to open opf file: %w", err)
	}
	defer rcOPF.Close()

	opfData, err := io.ReadAll(rcOPF)
	if err != nil {
		return fmt.Errorf("failed to read opf file: %w", err)
	}

	var opf opfPackage
	if err := xml.Unmarshal(opfData, &opf); err != nil {
		return fmt.Errorf("failed to unmarshal opf file: %w", err)
	}

	coverHref := findCoverHref(&opf)
	if coverHref == "" {
		return extractCoverByNameFallback(&r.Reader, destPath)
	}

	coverZipPath := path.Clean(path.Join(path.Dir(packageDocPath), coverHref))
	coverFile := findZipEntryCaseInsensitive(&r.Reader, coverZipPath)
	if coverFile == nil {
		return extractCoverByNameFallback(&r.Reader, destPath)
	}

	return extractZipEntry(coverFile, destPath)
}
