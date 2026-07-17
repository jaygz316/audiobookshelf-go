package scanner

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"

	idb "audiobookshelf/internal/db"
	log "audiobookshelf/internal/logger"
	"audiobookshelf/internal/metadata"
)

func parseEbookMetadata(dbConn *sql.DB, itemID string, ebookFiles []FileItem, meta *GroupMetadata, scannerFindCovers bool, itemPath string) {
	var eb FileItem
	for _, e := range ebookFiles {
		if strings.ToLower(e.Extension) == ".epub" {
			eb = e
			break
		}
	}
	if eb.Path == "" {
		eb = ebookFiles[0]
	}

	meta.EbookFile = map[string]interface{}{
		"ebookFormat": strings.ToLower(strings.TrimPrefix(eb.Extension, ".")),
		"metadata": map[string]interface{}{
			"path":     eb.Path,
			"relPath":  eb.RelPath,
			"filename": eb.Name,
			"ext":      eb.Extension,
			"size":     eb.Size,
			"mtime":    eb.MtimeMs,
			"ctime":    eb.CtimeMs,
		},
	}

	// Parse metadata from ebook using internal/metadata
	var parsed *metadata.EbookMetadata
	var err error
	log.Printf("[Scanner] [%s] Extracting ebook metadata from: %s", itemPath, eb.Path)
	if strings.ToLower(eb.Extension) == ".epub" {
		parsed, err = metadata.ExtractEpubMetadata(context.Background(), eb.Path)
	} else if ext := strings.ToLower(eb.Extension); ext == ".cbz" || ext == ".cbr" || ext == ".pdf" {
		parsed, err = metadata.ExtractComicMetadata(context.Background(), eb.Path)
	}
	if err != nil {
		log.Printf("[Scanner] [%s] Extracting ebook metadata failed: %v", itemPath, err)
	}
	if err == nil && parsed != nil {
		if meta.Title == "" && parsed.Title != "" {
			meta.Title = parsed.Title
		}
		if len(meta.Authors) == 0 && parsed.Author != "" {
			meta.Authors = parseNameString(parsed.Author)
		}
		if meta.Publisher == "" && parsed.Publisher != "" {
			meta.Publisher = parsed.Publisher
		}
		if meta.PublishedYear == "" && parsed.PublishedYear != "" {
			meta.PublishedYear = parsed.PublishedYear
		}
		if meta.Description == "" && parsed.Description != "" {
			meta.Description = parsed.Description
		}
		if meta.Language == "" && parsed.Language != "" {
			meta.Language = parsed.Language
		}
		if meta.ISBN == "" && parsed.ISBN != "" {
			meta.ISBN = parsed.ISBN
		}
		if len(meta.Chapters) == 0 && len(parsed.Chapters) > 0 {
			var chs []Chapter
			for _, c := range parsed.Chapters {
				chs = append(chs, Chapter{
					ID:    c.ID,
					Title: c.Title,
				})
			}
			meta.Chapters = chs
		}
	}

	// Extract cover from ebook if no cover image was found in the folder
	if scannerFindCovers && meta.CoverPath == "" {
		metadataCoverWithItem := true
		if dbConn != nil {
			if settings, err := idb.GetServerSettings(dbConn); err == nil && settings != nil {
				metadataCoverWithItem = settings.MetadataCoverWithItem
			}
		}

		var destCover string
		if metadataCoverWithItem {
			destCover = filepath.Join(filepath.Dir(eb.Path), "cover.jpg")
		} else {
			itemDir := filepath.Join(MetadataPath, "items", itemID)
			_ = os.MkdirAll(itemDir, 0755)
			destCover = filepath.Join(itemDir, "cover.jpg")
		}

		var extractErr error
		log.Printf("[Scanner] [%s] Extracting ebook cover from: %s to %s", itemPath, eb.Path, destCover)
		if strings.ToLower(eb.Extension) == ".epub" {
			extractErr = metadata.ExtractEpubCover(context.Background(), eb.Path, destCover)
		} else if ext := strings.ToLower(eb.Extension); ext == ".cbz" || ext == ".cbr" || ext == ".pdf" {
			extractErr = metadata.ExtractComicCover(context.Background(), eb.Path, destCover)
		}
		if extractErr == nil {
			meta.CoverPath = destCover
		} else {
			log.Printf("[Scanner] [%s] Extracting ebook cover failed: %v", itemPath, extractErr)
		}
	}
}
