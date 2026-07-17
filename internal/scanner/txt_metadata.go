package scanner

import (
	"os"
	"strings"

	log "audiobookshelf/internal/logger"
)

func parseTxtFilesMetadata(descFile, readerFile string, meta *GroupMetadata, itemPath string) {
	if descFile != "" {
		log.Printf("[Scanner] [%s] Reading description file: %s", itemPath, descFile)
		if data, err := os.ReadFile(descFile); err == nil && len(data) > 0 {
			meta.Description = strings.TrimSpace(string(data))
		} else if err != nil {
			log.Printf("[Scanner] [%s] Reading description file failed: %v", itemPath, err)
		}
	}
	if readerFile != "" {
		log.Printf("[Scanner] [%s] Reading reader file: %s", itemPath, readerFile)
		if data, err := os.ReadFile(readerFile); err == nil && len(data) > 0 {
			lines := strings.Split(string(data), "\n")
			if len(lines) > 0 && strings.TrimSpace(lines[0]) != "" {
				meta.Narrators = parseNameString(strings.TrimSpace(lines[0]))
			}
		} else if err != nil {
			log.Printf("[Scanner] [%s] Reading reader file failed: %v", itemPath, err)
		}
	}
}
