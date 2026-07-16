package feed

import (
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"audiobookshelf/internal/utils"
)

// Internal XML mapping structures
type rss struct {
	XMLName    xml.Name `xml:"rss"`
	Version    string   `xml:"version,attr"`
	ITunes     string   `xml:"xmlns:itunes,attr"`
	Podcast    string   `xml:"xmlns:podcast,attr"`
	GooglePlay string   `xml:"xmlns:googleplay,attr"`
	Channel    channel  `xml:"channel"`
}

type channel struct {
	Title          string       `xml:"title"`
	Description    string       `xml:"description"`
	Generator      string       `xml:"generator"`
	SiteURL        string       `xml:"link"`
	Language       string       `xml:"language,omitempty"`
	ITunesAuthor   string       `xml:"itunes:author,omitempty"`
	ITunesType     string       `xml:"itunes:type,omitempty"`
	ITunesExplicit string       `xml:"itunes:explicit,omitempty"`
	ITunesSummary  *cdata       `xml:"itunes:summary,omitempty"`
	ITunesImage    *itunesImage `xml:"itunes:image,omitempty"`
	ITunesOwner    *itunesOwner `xml:"itunes:owner,omitempty"`
	Image          *image       `xml:"image,omitempty"`
	Items          []item       `xml:"item"`
}

type cdata struct {
	Value string `xml:",cdata"`
}

type itunesImage struct {
	Href string `xml:"href,attr"`
}

type itunesOwner struct {
	Name  string `xml:"itunes:name,omitempty"`
	Email string `xml:"itunes:email,omitempty"`
}

type image struct {
	URL   string `xml:"url"`
	Title string `xml:"title"`
	Link  string `xml:"link"`
}

type item struct {
	Title             string    `xml:"title"`
	Description       string    `xml:"description,omitempty"`
	URL               string    `xml:"link,omitempty"`
	GUID              string    `xml:"guid,omitempty"`
	Author            string    `xml:"author,omitempty"`
	PubDate           string    `xml:"pubDate,omitempty"`
	Enclosure         enclosure `xml:"enclosure"`
	ITunesAuthor      string    `xml:"itunes:author,omitempty"`
	ITunesDuration    int       `xml:"itunes:duration,omitempty"`
	ITunesExplicit    string    `xml:"itunes:explicit,omitempty"`
	ITunesEpisodeType string    `xml:"itunes:episodeType,omitempty"`
	ITunesSeason      string    `xml:"itunes:season,omitempty"`
	ITunesEpisode     string    `xml:"itunes:episode,omitempty"`
	ITunesSummary     *cdata    `xml:"itunes:summary,omitempty"`
}

type enclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}

// ServeRSSFeed creates an HTTP handler returning the RSS XML podcast representation of a library item, playlist, collection, or series.
func (m *FeedManager) ServeRSSFeed(slug string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Reconstruct host prefix
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		host := r.Host
		if xfh := r.Header.Get("X-Forwarded-Host"); xfh != "" {
			host = xfh
		}
		hostPrefix := scheme + "://" + host

		ctx := r.Context()
		var entityID string
		var entityType string
		err := m.db.QueryRowContext(ctx, "SELECT entityId, type FROM feeds WHERE id = ?", slug).Scan(&entityID, &entityType)

		var itemID string
		if err == nil {
			itemID = entityID
		} else {
			// Fallback: check if slug itself is a valid playlist ID, collection ID, series ID, podcast ID, or book ID
			itemID = slug
			var exists int
			if m.db.QueryRowContext(ctx, "SELECT 1 FROM playlists WHERE id = ?", slug).Scan(&exists) == nil {
				entityType = "playlist"
			} else if m.db.QueryRowContext(ctx, "SELECT 1 FROM collections WHERE id = ?", slug).Scan(&exists) == nil {
				entityType = "collection"
			} else if m.db.QueryRowContext(ctx, "SELECT 1 FROM series WHERE id = ?", slug).Scan(&exists) == nil {
				entityType = "series"
			} else {
				// Must be a library item. Let's query its mediaType from libraryItems
				var mediaType string
				err := m.db.QueryRowContext(ctx, "SELECT mediaType FROM libraryItems WHERE id = ?", slug).Scan(&mediaType)
				if err == nil {
					entityType = mediaType // "book" or "podcast"
				} else {
					http.NotFound(w, r)
					return
				}
			}
		}

		// Route based on sub-path
		if strings.Contains(path, "/cover") {
			m.serveFeedCover(w, r, itemID, entityType)
			return
		}

		if strings.Contains(path, "/item/") {
			m.serveFeedItem(w, r, itemID, entityType)
			return
		}

		m.serveFeedXML(w, r, itemID, slug, hostPrefix, entityType)
	}
}

// Cover serving
func (m *FeedManager) serveFeedCover(w http.ResponseWriter, r *http.Request, itemID string, entityType string) {
	ctx := r.Context()

	var coverPath string
	if entityType == "playlist" {
		// PORT: Legacy behavior resolves playlist cover using the first available item cover
		rows, err := m.db.QueryContext(ctx, `
			SELECT mediaItemId, mediaItemType 
			FROM playlistMediaItems 
			WHERE playlistId = ? 
			ORDER BY "order" ASC
		`, itemID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var mediaItemID, mediaItemType string
				if err := rows.Scan(&mediaItemID, &mediaItemType); err == nil {
					if mediaItemType == "book" {
						var bookCover sql.NullString
						if err := m.db.QueryRowContext(ctx, "SELECT coverPath FROM books WHERE id = ?", mediaItemID).Scan(&bookCover); err == nil && bookCover.Valid && bookCover.String != "" {
							coverPath = bookCover.String
							break
						}
					} else if mediaItemType == "podcastEpisode" {
						var podcastCover sql.NullString
						if err := m.db.QueryRowContext(ctx, `
							SELECT p.coverPath FROM podcasts p
							JOIN podcastEpisodes pe ON pe.podcastId = p.id
							WHERE pe.id = ?
						`, mediaItemID).Scan(&podcastCover); err == nil && podcastCover.Valid && podcastCover.String != "" {
							coverPath = podcastCover.String
							break
						}
					}
				}
			}
			_ = rows.Err()
		}
	} else if entityType == "collection" {
		// Query first available book in collection
		rows, err := m.db.QueryContext(ctx, `
			SELECT b.coverPath
			FROM collectionBooks cb
			JOIN books b ON cb.bookId = b.id
			WHERE cb.collectionId = ?
			ORDER BY cb."order" ASC
		`, itemID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var bCover sql.NullString
				if err := rows.Scan(&bCover); err == nil && bCover.Valid && bCover.String != "" {
					coverPath = bCover.String
					break
				}
			}
			_ = rows.Err()
		}
	} else if entityType == "series" {
		// Query first available book in series
		rows, err := m.db.QueryContext(ctx, `
			SELECT b.coverPath
			FROM bookSeries bs
			JOIN books b ON bs.bookId = b.id
			WHERE bs.seriesId = ?
			ORDER BY CAST(bs.sequence AS REAL) ASC, bs.sequence ASC
		`, itemID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var bCover sql.NullString
				if err := rows.Scan(&bCover); err == nil && bCover.Valid && bCover.String != "" {
					coverPath = bCover.String
					break
				}
			}
			_ = rows.Err()
		}
	} else {
		var mediaID string = itemID
		var mediaType string = entityType

		// If itemID is a libraryItem ID, resolve to mediaID
		var mID string
		if m.db.QueryRowContext(ctx, "SELECT mediaId FROM libraryItems WHERE id = ?", itemID).Scan(&mID) == nil {
			mediaID = mID
		}

		var cp sql.NullString
		if mediaType == "book" {
			_ = m.db.QueryRowContext(ctx, "SELECT coverPath FROM books WHERE id = ?", mediaID).Scan(&cp)
		} else if mediaType == "podcast" {
			_ = m.db.QueryRowContext(ctx, "SELECT coverPath FROM podcasts WHERE id = ?", mediaID).Scan(&cp)
		}
		if cp.Valid {
			coverPath = cp.String
		}
	}

	if coverPath == "" {
		http.NotFound(w, r)
		return
	}

	if !utils.IsSafeFilePath(m.db, m.metadataPath, coverPath) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	http.ServeFile(w, r, coverPath)
}

// Media file serving (handles HTTP range requests automatically via ServeFile)
func (m *FeedManager) serveFeedItem(w http.ResponseWriter, r *http.Request, itemID string, entityType string) {
	ctx := r.Context()
	reqPath := r.URL.Path

	itemIdx := strings.Index(reqPath, "/item/")
	if itemIdx == -1 {
		http.NotFound(w, r)
		return
	}
	sub := reqPath[itemIdx+len("/item/"):]
	parts := strings.Split(sub, "/")
	if len(parts) == 0 {
		http.NotFound(w, r)
		return
	}
	episodeID := parts[0]

	var filePath string
	var mimeType string

	if entityType == "playlist" {
		rows, err := m.db.QueryContext(ctx, `
			SELECT p.mediaItemId, p.mediaItemType, b.audioFiles
			FROM playlistMediaItems p
			LEFT JOIN books b ON p.mediaItemId = b.id AND p.mediaItemType = 'book'
			WHERE p.playlistId = ?
			ORDER BY p."order" ASC
		`, itemID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var mediaItemID, mediaItemType string
				var audioFilesJSON sql.NullString
				if err := rows.Scan(&mediaItemID, &mediaItemType, &audioFilesJSON); err == nil {
					if mediaItemType == "podcastEpisode" && mediaItemID == episodeID {
						var audioFileJSON string
						err := m.db.QueryRowContext(ctx, "SELECT audioFile FROM podcastEpisodes WHERE id = ?", episodeID).Scan(&audioFileJSON)
						if err == nil {
							var af audioFile
							if json.Unmarshal([]byte(audioFileJSON), &af) == nil {
								filePath = af.Metadata.Path
								mimeType = af.MimeType
								break
							}
						}
					} else if mediaItemType == "book" && audioFilesJSON.Valid {
						var tracks []audiobookTrack
						if json.Unmarshal([]byte(audioFilesJSON.String), &tracks) == nil {
							for _, t := range tracks {
								if t.Exclude {
									continue
								}
								// PORT: Deterministic MD5 hash to uniquely identify tracks without database state
								trackID := computeMD5(itemID + "_" + mediaItemID + "_" + t.Metadata.Path)
								if trackID == episodeID {
									filePath = t.Metadata.Path
									mimeType = t.MimeType
									break
								}
							}
						}
						if filePath != "" {
							break
						}
					}
				}
			}
			_ = rows.Err()
		}
	} else if entityType == "collection" {
		// Fetch all books in collection
		rows, err := m.db.QueryContext(ctx, `
			SELECT b.id, b.audioFiles
			FROM collectionBooks cb
			JOIN books b ON cb.bookId = b.id
			WHERE cb.collectionId = ?
			ORDER BY cb."order" ASC
		`, itemID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var mediaItemID string
				var audioFilesJSON sql.NullString
				if err := rows.Scan(&mediaItemID, &audioFilesJSON); err == nil && audioFilesJSON.Valid {
					var tracks []audiobookTrack
					if json.Unmarshal([]byte(audioFilesJSON.String), &tracks) == nil {
						for _, t := range tracks {
							if t.Exclude {
								continue
							}
							trackID := computeMD5(itemID + "_" + mediaItemID + "_" + t.Metadata.Path)
							if trackID == episodeID {
								filePath = t.Metadata.Path
								mimeType = t.MimeType
								break
							}
						}
					}
					if filePath != "" {
						break
					}
				}
			}
			_ = rows.Err()
		}
	} else if entityType == "series" {
		// Fetch all books in series
		rows, err := m.db.QueryContext(ctx, `
			SELECT b.id, b.audioFiles
			FROM bookSeries bs
			JOIN books b ON bs.bookId = b.id
			WHERE bs.seriesId = ?
			ORDER BY CAST(bs.sequence AS REAL) ASC, bs.sequence ASC
		`, itemID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var mediaItemID string
				var audioFilesJSON sql.NullString
				if err := rows.Scan(&mediaItemID, &audioFilesJSON); err == nil && audioFilesJSON.Valid {
					var tracks []audiobookTrack
					if json.Unmarshal([]byte(audioFilesJSON.String), &tracks) == nil {
						for _, t := range tracks {
							if t.Exclude {
								continue
							}
							trackID := computeMD5(itemID + "_" + mediaItemID + "_" + t.Metadata.Path)
							if trackID == episodeID {
								filePath = t.Metadata.Path
								mimeType = t.MimeType
								break
							}
						}
					}
					if filePath != "" {
						break
					}
				}
			}
			_ = rows.Err()
		}
	} else {
		var mediaID string = itemID
		var mediaType string = entityType

		// If itemID is a libraryItem ID, resolve to mediaID
		var mID string
		if m.db.QueryRowContext(ctx, "SELECT mediaId FROM libraryItems WHERE id = ?", itemID).Scan(&mID) == nil {
			mediaID = mID
		}

		if mediaType == "podcast" {
			var audioFileJSON string
			err := m.db.QueryRowContext(ctx, `
				SELECT audioFile FROM podcastEpisodes 
				WHERE id = ? AND podcastId = ?
			`, episodeID, mediaID).Scan(&audioFileJSON)
			if err == nil {
				var af audioFile
				if json.Unmarshal([]byte(audioFileJSON), &af) == nil {
					filePath = af.Metadata.Path
					mimeType = af.MimeType
				}
			}
		} else if mediaType == "book" {
			var audioFilesJSON string
			err := m.db.QueryRowContext(ctx, "SELECT audioFiles FROM books WHERE id = ?", mediaID).Scan(&audioFilesJSON)
			if err == nil {
				var tracks []audiobookTrack
				if json.Unmarshal([]byte(audioFilesJSON), &tracks) == nil {
					for _, t := range tracks {
						if t.Exclude {
							continue
						}
						trackID := computeMD5(t.Metadata.Path)
						if trackID == episodeID {
							filePath = t.Metadata.Path
							mimeType = t.MimeType
							break
						}
					}
				}
			}
		}
	}

	if filePath == "" {
		http.NotFound(w, r)
		return
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}

	if mimeType != "" {
		w.Header().Set("Content-Type", mimeType)
	}

	if !utils.IsSafeFilePath(m.db, m.metadataPath, filePath) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	http.ServeFile(w, r, filePath)
}

// Serve podcast feed XML representation
func (m *FeedManager) serveFeedXML(w http.ResponseWriter, r *http.Request, itemID string, slug string, hostPrefix string, entityType string) {
	ctx := r.Context()

	reqPath := r.URL.Path
	feedIdx := strings.Index(reqPath, "/feed/")
	var pathPrefix string
	if feedIdx != -1 {
		pathPrefix = reqPath[:feedIdx] + "/feed/" + slug
	} else {
		pathPrefix = "/feed/" + slug
	}
	feedBaseURL := hostPrefix + pathPrefix

	var rssFeed rss
	rssFeed.Version = "2.0"
	rssFeed.ITunes = "http://www.itunes.com/dtds/podcast-1.0.dtd"
	rssFeed.Podcast = "https://podcastindex.org/namespace/1.0"
	rssFeed.GooglePlay = "http://www.google.com/schemas/play-podcasts/1.0"

	var feedChannel channel
	feedChannel.Generator = "Audiobookshelf"
	feedChannel.Language = "en"
	feedChannel.ITunesType = "serial"

	if entityType == "playlist" {
		var playlistName string
		var playlistDesc sql.NullString
		_ = m.db.QueryRowContext(ctx, "SELECT name, description FROM playlists WHERE id = ?", itemID).Scan(&playlistName, &playlistDesc)

		feedChannel.Title = playlistName
		if playlistDesc.Valid {
			feedChannel.Description = playlistDesc.String
			feedChannel.ITunesSummary = &cdata{Value: playlistDesc.String}
		}
		feedChannel.SiteURL = hostPrefix + "/playlist/" + itemID
		feedChannel.Image = &image{
			URL:   feedBaseURL + "/cover",
			Title: playlistName,
			Link:  feedChannel.SiteURL,
		}
		feedChannel.ITunesImage = &itunesImage{Href: feedBaseURL + "/cover"}

		rows, err := m.db.QueryContext(ctx, `
			SELECT mediaItemId, mediaItemType 
			FROM playlistMediaItems 
			WHERE playlistId = ? 
			ORDER BY "order" ASC
		`, itemID)
		if err == nil {
			type playlistItem struct {
				mediaItemID   string
				mediaItemType string
			}
			var items []playlistItem
			var bookIDs []interface{}
			var epIDs []interface{}

			defer rows.Close()
			for rows.Next() {
				var it playlistItem
				if err := rows.Scan(&it.mediaItemID, &it.mediaItemType); err == nil {
					items = append(items, it)
					if it.mediaItemType == "book" {
						bookIDs = append(bookIDs, it.mediaItemID)
					} else if it.mediaItemType == "podcastEpisode" {
						epIDs = append(epIDs, it.mediaItemID)
					}
				}
			}
			_ = rows.Err()

			type bookData struct {
				title       sql.NullString
				desc        sql.NullString
				explicit    int
				audioFiles  string
				chapters    string
				liCreatedAt string
			}
			bookMap := make(map[string]bookData)

			if len(bookIDs) > 0 {
				query := `
					SELECT b.id, b.title, b.description, b.explicit, b.audioFiles, b.chapters, li.createdAt
					FROM books b
					JOIN libraryItems li ON li.mediaId = b.id AND li.mediaType = 'book'
					WHERE b.id IN (?` + strings.Repeat(",?", len(bookIDs)-1) + `)
				`
				if bookRows, err := m.db.QueryContext(ctx, query, bookIDs...); err == nil {
					defer bookRows.Close()
					for bookRows.Next() {
						var id string
						var bd bookData
						if err := bookRows.Scan(&id, &bd.title, &bd.desc, &bd.explicit, &bd.audioFiles, &bd.chapters, &bd.liCreatedAt); err == nil {
							bookMap[id] = bd
						}
					}
				}
			}

			type epPodcastData struct {
				ep       *podcastEpData
				author   sql.NullString
				explicit int
			}
			epMap := make(map[string]epPodcastData)

			if len(epIDs) > 0 {
				for _, epID := range epIDs {
					idStr := epID.(string)
					ep, err := queryPodcastEpisode(ctx, m.db, idStr)
					if err == nil {
						epMap[idStr] = epPodcastData{ep: ep}
					}
				}

				query := `
					SELECT pe.id, p.author, p.explicit
					FROM podcasts p
					JOIN podcastEpisodes pe ON pe.podcastId = p.id
					WHERE pe.id IN (?` + strings.Repeat(",?", len(epIDs)-1) + `)
				`
				if pRows, err := m.db.QueryContext(ctx, query, epIDs...); err == nil {
					defer pRows.Close()
					for pRows.Next() {
						var epID string
						var author sql.NullString
						var explicit int
						if err := pRows.Scan(&epID, &author, &explicit); err == nil {
							if data, ok := epMap[epID]; ok {
								data.author = author
								data.explicit = explicit
								epMap[epID] = data
							}
						}
					}
				}
			}

			for _, it := range items {
				mediaItemID := it.mediaItemID
				mediaItemType := it.mediaItemType

				if mediaItemType == "podcastEpisode" {
					if epData, ok := epMap[mediaItemID]; ok {
						ep := epData.ep
						var af audioFile
						_ = json.Unmarshal([]byte(ep.AudioFile), &af)

						itemVal := item{
							Title:        ep.Title,
							Description:  ep.Description,
							URL:          hostPrefix + "/item/" + ep.ID,
							GUID:         feedBaseURL + "/item/" + ep.ID + "/media",
							Author:       epData.author.String,
							ITunesAuthor: epData.author.String,
							PubDate:      formatPubDate(ep.PubDate),
							Enclosure: enclosure{
								URL:    feedBaseURL + "/item/" + ep.ID + "/media" + af.Metadata.Ext,
								Length: af.Metadata.Size,
								Type:   af.MimeType,
							},
							ITunesDuration: int(math.Round(af.Duration)),
						}
						if epData.explicit != 0 {
							itemVal.ITunesExplicit = "yes"
						} else {
							itemVal.ITunesExplicit = "no"
						}
						if ep.Season != "" {
							itemVal.ITunesSeason = ep.Season
						}
						if ep.Episode != "" {
							itemVal.ITunesEpisode = ep.Episode
						}
						if ep.EpisodeType != "" {
							itemVal.ITunesEpisodeType = ep.EpisodeType
						}
						if ep.Description != "" {
							itemVal.ITunesSummary = &cdata{Value: ep.Description}
						}
						feedChannel.Items = append(feedChannel.Items, itemVal)
					}
				} else if mediaItemType == "book" {
					if bd, ok := bookMap[mediaItemID]; ok {
						var tracks []audiobookTrack
						_ = json.Unmarshal([]byte(bd.audioFiles), &tracks)
						var chapters []audiobookChapter
						_ = json.Unmarshal([]byte(bd.chapters), &chapters)

						liCreatedAt, _ := time.Parse(time.RFC3339, bd.liCreatedAt)
						if liCreatedAt.IsZero() {
							liCreatedAt = time.Now()
						}

						useChapterTitles := checkUseChapterTitles(tracks, chapters)

						for i, t := range tracks {
							if t.Exclude {
								continue
							}
							trackID := computeMD5(itemID + "_" + mediaItemID + "_" + t.Metadata.Path)
							ext := filepath.Ext(t.Metadata.Filename)

							title := strings.TrimSuffix(t.Metadata.Filename, ext)
							if len(tracks) == 1 {
								title = bd.title.String
							} else if useChapterTitles {
								for _, ch := range chapters {
									if math.Abs(ch.Start-t.StartOffset) < 1.0 {
										title = ch.Title
										break
									}
								}
							}

							pubDate := liCreatedAt.Add(time.Duration(i) * time.Minute).UTC().Format(time.RFC1123Z)

							itemVal := item{
								Title:       title,
								Description: bd.desc.String,
								URL:         hostPrefix + "/item/" + mediaItemID,
								GUID:        feedBaseURL + "/item/" + trackID + "/media",
								PubDate:     pubDate,
								Enclosure: enclosure{
									URL:    feedBaseURL + "/item/" + trackID + "/media" + ext,
									Length: t.Metadata.Size,
									Type:   t.MimeType,
								},
								ITunesDuration: int(math.Round(t.Duration)),
							}
							if bd.explicit != 0 {
								itemVal.ITunesExplicit = "yes"
							} else {
								itemVal.ITunesExplicit = "no"
							}
							if bd.desc.String != "" {
								itemVal.ITunesSummary = &cdata{Value: bd.desc.String}
							}
							feedChannel.Items = append(feedChannel.Items, itemVal)
						}
					}
				}
			}
		}
	} else if entityType == "collection" {
		var collName string
		var collDesc sql.NullString
		err := m.db.QueryRowContext(ctx, "SELECT name, description FROM collections WHERE id = ?", itemID).Scan(&collName, &collDesc)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		feedChannel.Title = collName
		if collDesc.Valid {
			feedChannel.Description = collDesc.String
			feedChannel.ITunesSummary = &cdata{Value: collDesc.String}
		}
		feedChannel.SiteURL = hostPrefix + "/collection/" + itemID
		feedChannel.Image = &image{
			URL:   feedBaseURL + "/cover",
			Title: collName,
			Link:  feedChannel.SiteURL,
		}
		feedChannel.ITunesImage = &itunesImage{Href: feedBaseURL + "/cover"}

		// Query all books in the collection
		rows, err := m.db.QueryContext(ctx, `
			SELECT b.id, b.title, b.description, b.explicit, b.audioFiles, b.chapters, li.createdAt
			FROM collectionBooks cb
			JOIN books b ON cb.bookId = b.id
			JOIN libraryItems li ON li.mediaId = b.id AND li.mediaType = 'book'
			WHERE cb.collectionId = ?
			ORDER BY cb."order" ASC
		`, itemID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var mediaItemID string
				var title, desc sql.NullString
				var explicit int
				var audioFilesJSON, chaptersJSON, liCreatedAtStr string
				if err := rows.Scan(&mediaItemID, &title, &desc, &explicit, &audioFilesJSON, &chaptersJSON, &liCreatedAtStr); err == nil {
					var tracks []audiobookTrack
					_ = json.Unmarshal([]byte(audioFilesJSON), &tracks)
					var chapters []audiobookChapter
					_ = json.Unmarshal([]byte(chaptersJSON), &chapters)

					liCreatedAt, _ := time.Parse(time.RFC3339, liCreatedAtStr)
					if liCreatedAt.IsZero() {
						liCreatedAt = time.Now()
					}

					useChapterTitles := checkUseChapterTitles(tracks, chapters)

					for i, t := range tracks {
						if t.Exclude {
							continue
						}
						trackID := computeMD5(itemID + "_" + mediaItemID + "_" + t.Metadata.Path)
						ext := filepath.Ext(t.Metadata.Filename)

						epTitle := strings.TrimSuffix(t.Metadata.Filename, ext)
						if len(tracks) == 1 {
							epTitle = title.String
						} else if useChapterTitles {
							for _, ch := range chapters {
								if math.Abs(ch.Start-t.StartOffset) < 1.0 {
									epTitle = ch.Title
									break
								}
							}
						}

						pubDate := liCreatedAt.Add(time.Duration(i) * time.Minute).UTC().Format(time.RFC1123Z)

						itemVal := item{
							Title:       epTitle,
							Description: desc.String,
							URL:         hostPrefix + "/item/" + mediaItemID,
							GUID:        feedBaseURL + "/item/" + trackID + "/media",
							PubDate:     pubDate,
							Enclosure: enclosure{
								URL:    feedBaseURL + "/item/" + trackID + "/media" + ext,
								Length: t.Metadata.Size,
								Type:   t.MimeType,
							},
							ITunesDuration: int(math.Round(t.Duration)),
						}
						if explicit != 0 {
							itemVal.ITunesExplicit = "yes"
						} else {
							itemVal.ITunesExplicit = "no"
						}
						if desc.String != "" {
							itemVal.ITunesSummary = &cdata{Value: desc.String}
						}
						feedChannel.Items = append(feedChannel.Items, itemVal)
					}
				}
			}
			_ = rows.Err()
		}
	} else if entityType == "series" {
		var seriesName string
		var seriesDesc sql.NullString
		err := m.db.QueryRowContext(ctx, "SELECT name, description FROM series WHERE id = ?", itemID).Scan(&seriesName, &seriesDesc)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		feedChannel.Title = seriesName
		if seriesDesc.Valid {
			feedChannel.Description = seriesDesc.String
			feedChannel.ITunesSummary = &cdata{Value: seriesDesc.String}
		}
		feedChannel.SiteURL = hostPrefix + "/series/" + itemID
		feedChannel.Image = &image{
			URL:   feedBaseURL + "/cover",
			Title: seriesName,
			Link:  feedChannel.SiteURL,
		}
		feedChannel.ITunesImage = &itunesImage{Href: feedBaseURL + "/cover"}

		// Query all books in the series
		rows, err := m.db.QueryContext(ctx, `
			SELECT b.id, b.title, b.description, b.explicit, b.audioFiles, b.chapters, li.createdAt, bs.sequence
			FROM bookSeries bs
			JOIN books b ON bs.bookId = b.id
			JOIN libraryItems li ON li.mediaId = b.id AND li.mediaType = 'book'
			WHERE bs.seriesId = ?
			ORDER BY CAST(bs.sequence AS REAL) ASC, bs.sequence ASC
		`, itemID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var mediaItemID string
				var title, desc sql.NullString
				var explicit int
				var audioFilesJSON, chaptersJSON, liCreatedAtStr, sequence string
				if err := rows.Scan(&mediaItemID, &title, &desc, &explicit, &audioFilesJSON, &chaptersJSON, &liCreatedAtStr, &sequence); err == nil {
					var tracks []audiobookTrack
					_ = json.Unmarshal([]byte(audioFilesJSON), &tracks)
					var chapters []audiobookChapter
					_ = json.Unmarshal([]byte(chaptersJSON), &chapters)

					liCreatedAt, _ := time.Parse(time.RFC3339, liCreatedAtStr)
					if liCreatedAt.IsZero() {
						liCreatedAt = time.Now()
					}

					useChapterTitles := checkUseChapterTitles(tracks, chapters)

					for i, t := range tracks {
						if t.Exclude {
							continue
						}
						trackID := computeMD5(itemID + "_" + mediaItemID + "_" + t.Metadata.Path)
						ext := filepath.Ext(t.Metadata.Filename)

						epTitle := strings.TrimSuffix(t.Metadata.Filename, ext)
						if len(tracks) == 1 {
							epTitle = title.String
							if sequence != "" {
								epTitle = fmt.Sprintf("Book %s - %s", sequence, epTitle)
							}
						} else if useChapterTitles {
							for _, ch := range chapters {
								if math.Abs(ch.Start-t.StartOffset) < 1.0 {
									epTitle = ch.Title
									break
								}
							}
						}

						pubDate := liCreatedAt.Add(time.Duration(i) * time.Minute).UTC().Format(time.RFC1123Z)

						itemVal := item{
							Title:       epTitle,
							Description: desc.String,
							URL:         hostPrefix + "/item/" + mediaItemID,
							GUID:        feedBaseURL + "/item/" + trackID + "/media",
							PubDate:     pubDate,
							Enclosure: enclosure{
								URL:    feedBaseURL + "/item/" + trackID + "/media" + ext,
								Length: t.Metadata.Size,
								Type:   t.MimeType,
							},
							ITunesDuration: int(math.Round(t.Duration)),
						}
						if explicit != 0 {
							itemVal.ITunesExplicit = "yes"
						} else {
							itemVal.ITunesExplicit = "no"
						}
						if desc.String != "" {
							itemVal.ITunesSummary = &cdata{Value: desc.String}
						}
						feedChannel.Items = append(feedChannel.Items, itemVal)
					}
				}
			}
			_ = rows.Err()
		}
	} else {
		var mediaType, mediaID, liCreatedAtStr string
		err := m.db.QueryRowContext(ctx, "SELECT mediaType, mediaId, createdAt FROM libraryItems WHERE id = ?", itemID).Scan(&mediaType, &mediaID, &liCreatedAtStr)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		liCreatedAt, _ := time.Parse(time.RFC3339, liCreatedAtStr)
		if liCreatedAt.IsZero() {
			liCreatedAt = time.Now()
		}

		if mediaType == "podcast" {
			var title, author, description, language, podcastType sql.NullString
			var explicit int
			err := m.db.QueryRowContext(ctx, `
				SELECT title, author, description, language, podcastType, explicit 
				FROM podcasts WHERE id = ?
			`, mediaID).Scan(&title, &author, &description, &language, &podcastType, &explicit)
			if err != nil {
				http.NotFound(w, r)
				return
			}

			feedChannel.Title = title.String
			feedChannel.Description = description.String
			if description.Valid && description.String != "" {
				feedChannel.ITunesSummary = &cdata{Value: description.String}
			}
			if language.Valid && language.String != "" {
				feedChannel.Language = language.String
			}
			if podcastType.Valid && podcastType.String != "" {
				feedChannel.ITunesType = podcastType.String
			}
			feedChannel.SiteURL = hostPrefix + "/item/" + itemID
			feedChannel.Image = &image{
				URL:   feedBaseURL + "/cover",
				Title: title.String,
				Link:  feedChannel.SiteURL,
			}
			feedChannel.ITunesImage = &itunesImage{Href: feedBaseURL + "/cover"}
			feedChannel.ITunesAuthor = author.String
			if explicit != 0 {
				feedChannel.ITunesExplicit = "yes"
			} else {
				feedChannel.ITunesExplicit = "no"
			}

			eps, err := queryPodcastEpisodes(ctx, m.db, mediaID)
			if err == nil {
				sortPodcastEpisodes(eps, feedChannel.ITunesType == "episodic")

				for _, ep := range eps {
					var af audioFile
					_ = json.Unmarshal([]byte(ep.AudioFile), &af)

					itemVal := item{
						Title:        ep.Title,
						Description:  ep.Description,
						URL:          hostPrefix + "/item/" + ep.ID,
						GUID:         feedBaseURL + "/item/" + ep.ID + "/media",
						Author:       author.String,
						ITunesAuthor: author.String,
						PubDate:      formatPubDate(ep.PubDate),
						Enclosure: enclosure{
							URL:    feedBaseURL + "/item/" + ep.ID + "/media" + af.Metadata.Ext,
							Length: af.Metadata.Size,
							Type:   af.MimeType,
						},
						ITunesDuration: int(math.Round(af.Duration)),
					}
					if explicit != 0 {
						itemVal.ITunesExplicit = "yes"
					} else {
						itemVal.ITunesExplicit = "no"
					}
					if ep.Season != "" {
						itemVal.ITunesSeason = ep.Season
					}
					if ep.Episode != "" {
						itemVal.ITunesEpisode = ep.Episode
					}
					if ep.EpisodeType != "" {
						itemVal.ITunesEpisodeType = ep.EpisodeType
					}
					if ep.Description != "" {
						itemVal.ITunesSummary = &cdata{Value: ep.Description}
					}
					feedChannel.Items = append(feedChannel.Items, itemVal)
				}
			}
		} else if mediaType == "book" {
			var title, description, language sql.NullString
			var explicit int
			var audioFilesJSON, chaptersJSON string
			err := m.db.QueryRowContext(ctx, `
				SELECT title, description, language, explicit, audioFiles, chapters 
				FROM books WHERE id = ?
			`, mediaID).Scan(&title, &description, &language, &explicit, &audioFilesJSON, &chaptersJSON)
			if err != nil {
				http.NotFound(w, r)
				return
			}

			var authorName sql.NullString
			_ = m.db.QueryRowContext(ctx, "SELECT authorNamesFirstLast FROM libraryItems WHERE id = ?", itemID).Scan(&authorName)

			feedChannel.Title = title.String
			feedChannel.Description = description.String
			if description.Valid && description.String != "" {
				feedChannel.ITunesSummary = &cdata{Value: description.String}
			}
			if language.Valid && language.String != "" {
				feedChannel.Language = language.String
			}
			feedChannel.SiteURL = hostPrefix + "/item/" + itemID
			feedChannel.Image = &image{
				URL:   feedBaseURL + "/cover",
				Title: title.String,
				Link:  feedChannel.SiteURL,
			}
			feedChannel.ITunesImage = &itunesImage{Href: feedBaseURL + "/cover"}
			feedChannel.ITunesAuthor = authorName.String
			if explicit != 0 {
				feedChannel.ITunesExplicit = "yes"
			} else {
				feedChannel.ITunesExplicit = "no"
			}

			var tracks []audiobookTrack
			_ = json.Unmarshal([]byte(audioFilesJSON), &tracks)
			var chapters []audiobookChapter
			_ = json.Unmarshal([]byte(chaptersJSON), &chapters)

			useChapterTitles := checkUseChapterTitles(tracks, chapters)

			for i, t := range tracks {
				if t.Exclude {
					continue
				}
				trackID := computeMD5(t.Metadata.Path)
				ext := filepath.Ext(t.Metadata.Filename)

				epTitle := strings.TrimSuffix(t.Metadata.Filename, ext)
				if len(tracks) == 1 {
					epTitle = title.String
				} else if useChapterTitles {
					for _, ch := range chapters {
						if math.Abs(ch.Start-t.StartOffset) < 1.0 {
							epTitle = ch.Title
							break
						}
					}
				}

				pubDate := liCreatedAt.Add(time.Duration(i) * time.Minute).UTC().Format(time.RFC1123Z)

				itemVal := item{
					Title:       epTitle,
					Description: description.String,
					URL:         hostPrefix + "/item/" + itemID,
					GUID:        feedBaseURL + "/item/" + trackID + "/media",
					PubDate:     pubDate,
					Enclosure: enclosure{
						URL:    feedBaseURL + "/item/" + trackID + "/media" + ext,
						Length: t.Metadata.Size,
						Type:   t.MimeType,
					},
					ITunesDuration: int(math.Round(t.Duration)),
				}
				if explicit != 0 {
					itemVal.ITunesExplicit = "yes"
				} else {
					itemVal.ITunesExplicit = "no"
				}
				if description.String != "" {
					itemVal.ITunesSummary = &cdata{Value: description.String}
				}
				feedChannel.Items = append(feedChannel.Items, itemVal)
			}
		} else {
			http.NotFound(w, r)
			return
		}
	}

	rssFeed.Channel = feedChannel

	xmlBytes, err := xml.MarshalIndent(rssFeed, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(xmlBytes)
}
