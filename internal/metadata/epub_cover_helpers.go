package metadata

import (
	"archive/zip"
	"errors"
	"path/filepath"
	"sort"
	"strings"
)

func findCoverHref(opf *opfPackage) string {
	var coverID string
	for _, m := range opf.Metadata.Meta {
		if m.Name == "cover" {
			coverID = m.Content
			break
		}
	}

	if coverID != "" {
		for _, item := range opf.Manifest.Items {
			if item.ID == coverID {
				return item.Href
			}
		}
	}

	for _, item := range opf.Manifest.Items {
		properties := strings.Split(item.Properties, " ")
		for _, p := range properties {
			if p == "cover-image" {
				return item.Href
			}
		}
	}

	for _, item := range opf.Manifest.Items {
		if strings.HasPrefix(item.MediaType, "image/") {
			return item.Href
		}
	}

	return ""
}

func extractCoverByNameFallback(r *zip.Reader, destPath string) error {
	var coverCandidates []string
	for _, f := range r.File {
		if !f.FileInfo().IsDir() {
			base := strings.ToLower(filepath.Base(f.Name))
			if base == "cover.jpg" || base == "cover.jpeg" || base == "cover.png" ||
				base == "folder.jpg" || base == "folder.jpeg" || base == "folder.png" {
				coverCandidates = append(coverCandidates, f.Name)
			}
		}
	}

	if len(coverCandidates) == 0 {
		return errors.New("no cover image found in epub zip archive")
	}

	sort.Slice(coverCandidates, func(i, j int) bool {
		iBase := strings.ToLower(filepath.Base(coverCandidates[i]))
		jBase := strings.ToLower(filepath.Base(coverCandidates[j]))
		if strings.HasPrefix(iBase, "cover") && strings.HasPrefix(jBase, "folder") {
			return true
		}
		if strings.HasPrefix(iBase, "folder") && strings.HasPrefix(jBase, "cover") {
			return false
		}
		return coverCandidates[i] < coverCandidates[j]
	})

	targetEntry := findZipEntryCaseInsensitive(r, coverCandidates[0])
	if targetEntry == nil {
		return errors.New("failed to locate fallback cover file inside zip")
	}

	return extractZipEntry(targetEntry, destPath)
}
