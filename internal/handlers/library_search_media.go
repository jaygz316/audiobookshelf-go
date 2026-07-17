package handlers

import (
	"database/sql"
	"encoding/json"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/utils"
)

func searchBooks(db *sql.DB, libraryID string, user *core.UserSession, q string, limit int) []map[string]interface{} {
	var bookResults []map[string]interface{} = []map[string]interface{}{}
	optsBooks := idb.GetFilteredLibraryItemsOptions{
		LibraryID: libraryID,
		User:      user,
		Search:    q,
		Limit:     limit,
		MediaType: "book",
	}
	bookItems, _, err := idb.GetFilteredLibraryItems(db, optsBooks)
	if err == nil {
		for _, item := range bookItems {
			bookResults = append(bookResults, map[string]interface{}{
				"libraryItem": item,
			})
		}
	}
	return bookResults
}

func searchPodcasts(db *sql.DB, libraryID string, user *core.UserSession, q string, limit int) []map[string]interface{} {
	var podcastResults []map[string]interface{} = []map[string]interface{}{}
	optsPodcasts := idb.GetFilteredLibraryItemsOptions{
		LibraryID: libraryID,
		User:      user,
		Search:    q,
		Limit:     limit,
		MediaType: "podcast",
	}
	podcastItems, _, err := idb.GetFilteredLibraryItems(db, optsPodcasts)
	if err == nil {
		for _, item := range podcastItems {
			podcastResults = append(podcastResults, map[string]interface{}{
				"libraryItem": item,
			})
		}
	}
	return podcastResults
}

func searchEpisodes(db *sql.DB, libraryID string, q string, limit int) []map[string]interface{} {
	var episodeResults []map[string]interface{} = []map[string]interface{}{}
	searchTerm := "%" + q + "%"
	epRows, err := db.Query(`
		SELECT pe.id, pe.title, pe.audioFile, pe.pubDate, pe.description, pe.season, pe.episode, pe.episodeType, pe.enclosureURL, pe.publishedAt, pe.podcastId, li.id
		FROM podcastEpisodes pe
		JOIN libraryItems li ON li.mediaId = pe.podcastId AND li.mediaType = 'podcast'
		WHERE li.libraryId = ? AND (pe.title LIKE ? OR pe.description LIKE ?)
		LIMIT ?
	`, libraryID, searchTerm, searchTerm, limit)
	if err == nil {
		defer epRows.Close()
		for epRows.Next() {
			var epID, epTitle, epPodcastID, liID string
			var epAudioFile, epPubDate, epDesc, epSeason, epEpisode, epEpType, epEncURL, epPubAt sql.NullString
			if err := epRows.Scan(&epID, &epTitle, &epAudioFile, &epPubDate, &epDesc, &epSeason, &epEpisode, &epEpType, &epEncURL, &epPubAt, &epPodcastID, &liID); err == nil {
				if minItem, err := idb.GetLibraryItemMinifiedByID(db, liID); err == nil {
					epMap := map[string]interface{}{
						"id":        epID,
						"title":     epTitle,
						"audioFile": utils.NullIfEmpty(epAudioFile.String),
					}
					if epPubDate.Valid {
						epMap["pubDate"] = epPubDate.String
					}
					if epDesc.Valid {
						epMap["description"] = epDesc.String
					}
					if epSeason.Valid {
						epMap["season"] = epSeason.String
					}
					if epEpisode.Valid {
						epMap["episode"] = epEpisode.String
					}
					if epEpType.Valid {
						epMap["episodeType"] = epEpType.String
					}
					if epEncURL.Valid {
						epMap["enclosureURL"] = epEncURL.String
					}
					if epPubAt.Valid {
						epMap["publishedAt"] = epPubAt.String
					}

					minItemBytes, _ := json.Marshal(minItem)
					var minItemMap map[string]interface{}
					if json.Unmarshal(minItemBytes, &minItemMap) == nil {
						minItemMap["recentEpisode"] = epMap
						episodeResults = append(episodeResults, map[string]interface{}{
							"libraryItem": minItemMap,
						})
					}
				}
			}
		}
	}
	return episodeResults
}
