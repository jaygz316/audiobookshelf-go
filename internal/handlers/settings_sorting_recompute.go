package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"strings"
)

// recomputeIgnorePrefixes asynchronously updates book, podcast, and series ignore prefix title columns
func recomputeIgnorePrefixes(db *sql.DB, prefixes []string) {
	log.Infof("[Prefix Recompute] Starting recompute with prefixes: %v", prefixes)
	recomputeBooksIgnorePrefixes(db, prefixes)
	recomputePodcastsIgnorePrefixes(db, prefixes)
	recomputeSeriesIgnorePrefixes(db, prefixes)
	log.Infof("[Prefix Recompute] Finished")
}

func recomputeBooksIgnorePrefixes(db *sql.DB, prefixes []string) {
	rows, err := db.Query("SELECT id, title, titleIgnorePrefix FROM books")
	if err != nil {
		log.Errorf("[Prefix Recompute] Failed to query books: %v", err)
		return
	}
	defer rows.Close()

	type bookUpdate struct {
		id        string
		newIgnore string
	}
	var updates []bookUpdate

	for rows.Next() {
		var id, title, currentIgnore string
		if err := rows.Scan(&id, &title, &currentIgnore); err != nil {
			log.Errorf("[Prefix Recompute] Failed to scan book: %v", err)
			continue
		}
		newIgnore := getTitleIgnorePrefixGo(title, prefixes)
		if newIgnore != currentIgnore {
			updates = append(updates, bookUpdate{id: id, newIgnore: newIgnore})
		}
	}
	if err := rows.Err(); err != nil {
		log.Errorf("[Prefix Recompute] Books query iteration error: %v", err)
	}
	rows.Close()

	for _, up := range updates {
		if _, err := db.Exec("UPDATE books SET titleIgnorePrefix = ? WHERE id = ?", up.newIgnore, up.id); err != nil {
			log.Errorf("[Prefix Recompute] Failed to update book %s: %v", up.id, err)
		}
	}
}

func recomputePodcastsIgnorePrefixes(db *sql.DB, prefixes []string) {
	rows, err := db.Query("SELECT id, title, titleIgnorePrefix FROM podcasts")
	if err != nil {
		log.Errorf("[Prefix Recompute] Failed to query podcasts: %v", err)
		return
	}
	defer rows.Close()

	type podcastUpdate struct {
		id        string
		newIgnore string
	}
	var updates []podcastUpdate

	for rows.Next() {
		var id, title, currentIgnore string
		if err := rows.Scan(&id, &title, &currentIgnore); err != nil {
			log.Errorf("[Prefix Recompute] Failed to scan podcast: %v", err)
			continue
		}
		newIgnore := getTitleIgnorePrefixGo(title, prefixes)
		if newIgnore != currentIgnore {
			updates = append(updates, podcastUpdate{id: id, newIgnore: newIgnore})
		}
	}
	if err := rows.Err(); err != nil {
		log.Errorf("[Prefix Recompute] Podcasts query iteration error: %v", err)
	}
	rows.Close()

	for _, up := range updates {
		if _, err := db.Exec("UPDATE podcasts SET titleIgnorePrefix = ? WHERE id = ?", up.newIgnore, up.id); err != nil {
			log.Errorf("[Prefix Recompute] Failed to update podcast %s: %v", up.id, err)
		}
	}
}

func recomputeSeriesIgnorePrefixes(db *sql.DB, prefixes []string) {
	rows, err := db.Query("SELECT id, name, nameIgnorePrefix FROM series")
	if err != nil {
		log.Errorf("[Prefix Recompute] Failed to query series: %v", err)
		return
	}
	defer rows.Close()

	type seriesUpdate struct {
		id        string
		newIgnore string
	}
	var updates []seriesUpdate

	for rows.Next() {
		var id, name, currentIgnore string
		if err := rows.Scan(&id, &name, &currentIgnore); err != nil {
			log.Errorf("[Prefix Recompute] Failed to scan series: %v", err)
			continue
		}
		newIgnore := getTitleIgnorePrefixGo(name, prefixes)
		if newIgnore != currentIgnore {
			updates = append(updates, seriesUpdate{id: id, newIgnore: newIgnore})
		}
	}
	if err := rows.Err(); err != nil {
		log.Errorf("[Prefix Recompute] Series query iteration error: %v", err)
	}
	rows.Close()

	for _, up := range updates {
		if _, err := db.Exec("UPDATE series SET nameIgnorePrefix = ? WHERE id = ?", up.newIgnore, up.id); err != nil {
			log.Errorf("[Prefix Recompute] Failed to update series %s: %v", up.id, err)
		}
	}
}

func getTitleIgnorePrefixGo(title string, prefixes []string) string {
	lowerTitle := strings.ToLower(title)
	for _, p := range prefixes {
		if strings.HasPrefix(lowerTitle, p+" ") {
			return title[len(p)+1:]
		}
	}
	return title
}
