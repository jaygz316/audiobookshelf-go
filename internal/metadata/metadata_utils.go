package metadata

import (
	"archive/zip"
	"fmt"
	"html"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var htmlTagRegex = regexp.MustCompile(`(?i)<[^>]*>`)
var yearRegex = regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
var supportedImages = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
	".gif":  true,
}

func findZipEntryCaseInsensitive(r *zip.Reader, targetPath string) *zip.File {
	cleaned := path.Clean(targetPath)
	for _, f := range r.File {
		if f.Name == cleaned {
			return f
		}
	}
	for _, f := range r.File {
		if strings.EqualFold(f.Name, cleaned) {
			return f
		}
	}
	return nil
}

func extractZipEntry(f *zip.File, destPath string) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("failed to open zip entry for extraction: %w", err)
	}
	defer rc.Close()

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create destination parent directory: %w", err)
	}

	destFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open destination file for cover extraction: %w", err)
	}
	defer destFile.Close()

	if _, err = io.Copy(destFile, rc); err != nil {
		return fmt.Errorf("failed to write cover image data: %w", err)
	}

	return nil
}

func stripAllTags(s string) string {
	s = html.UnescapeString(s)
	s = htmlTagRegex.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(s)
}

func isImageFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return supportedImages[ext]
}

func naturalLess(a, b string) bool {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		aChar := a[i]
		bChar := b[j]
		if isDigit(aChar) && isDigit(bChar) {
			aNumStr := ""
			for i < len(a) && isDigit(a[i]) {
				aNumStr += string(a[i])
				i++
			}
			bNumStr := ""
			for j < len(b) && isDigit(b[j]) {
				bNumStr += string(b[j])
				j++
			}
			aNum, _ := strconv.Atoi(aNumStr)
			bNum, _ := strconv.Atoi(bNumStr)
			if aNum != bNum {
				return aNum < bNum
			}
		} else {
			aLower := strings.ToLower(string(aChar))
			bLower := strings.ToLower(string(bChar))
			if aLower != bLower {
				return aLower < bLower
			}
			i++
			j++
		}
	}
	return len(a) < len(b)
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("failed to create destination parent directory: %w", err)
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open destination file: %w", err)
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return fmt.Errorf("failed to copy file data: %w", err)
	}
	return nil
}
