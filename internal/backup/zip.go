package backup

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func zipBackupContents(destZipPath, tempDBPath, metadataPath, id string, now time.Time) error {
	zipFile, err := os.Create(destZipPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zw := zip.NewWriter(zipFile)
	defer zw.Close()

	if err := addFileToZip(zw, tempDBPath, "absdatabase.sqlite"); err != nil {
		return err
	}

	createdAt := now.UnixNano() / int64(time.Millisecond)
	detailsString := fmt.Sprintf("%s\nsqlite\n%d\n2.8.0\n", id, createdAt)
	writer, err := zw.Create("details")
	if err != nil {
		return err
	}
	if _, err := writer.Write([]byte(detailsString)); err != nil {
		return err
	}

	if err := addDirToZip(zw, filepath.Join(metadataPath, "items"), "metadata-items"); err != nil {
		return err
	}

	if err := addDirToZip(zw, filepath.Join(metadataPath, "authors"), "metadata-authors"); err != nil {
		return err
	}

	// Verify Zip Writer Close error
	if err := zw.Close(); err != nil {
		return err
	}

	return zipFile.Close()
}

func addFileToZip(zw *zip.Writer, srcPath, destName string) error {
	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(fi)
	if err != nil {
		return err
	}
	header.Name = destName
	header.Method = zip.Deflate

	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(writer, file)
	return err
}

func addDirToZip(zw *zip.Writer, srcDir string, zipDirName string) error {
	if _, err := os.Stat(srcDir); err != nil {
		if os.IsNotExist(err) {
			return nil // Normal for fresh installs; skip walking
		}
		return err
	}

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		// Windows separator normalization: filepath.ToSlash
		header.Name = filepath.ToSlash(filepath.Join(zipDirName, rel))
		header.Method = zip.Deflate

		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		if _, err = io.Copy(writer, file); err != nil {
			return err
		}
		return nil
	})
}
