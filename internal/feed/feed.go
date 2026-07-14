package feed

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"audiobookshelf/internal/utils"
)

type FeedManager struct {
	db           *sql.DB
	metadataPath string
}

// NewFeedManager constructs an XML Feed manager.
func NewFeedManager(db *sql.DB, metadataPath string) *FeedManager {
	return &FeedManager{db: db, metadataPath: metadataPath}
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

// GenerateOPML generates an OPML XML payload mapping all podcasts inside a user's library.
func (m *FeedManager) GenerateOPML(ctx context.Context, userID, libraryID string) (string, error) {
	// 1. Check user access to the library
	ok, err := m.checkUserAccess(ctx, userID, libraryID)
	if err != nil {
		return "", fmt.Errorf("check user access: %w", err)
	}
	if !ok {
		return "", fmt.Errorf("user does not have access to library %s", libraryID)
	}

	// 2. Query all podcasts in this library
	rows, err := m.db.QueryContext(ctx, `
		SELECT p.title, p.feedURL, p.description, p.itunesPageURL, p.language
		FROM libraryItems li
		JOIN podcasts p ON li.mediaId = p.id AND li.mediaType = 'podcast'
		WHERE li.libraryId = ?
	`, libraryID)
	if err != nil {
		return "", fmt.Errorf("query podcasts in library: %w", err)
	}
	defer rows.Close()

	var outlines []opmlOutline
	for rows.Next() {
		var title sql.NullString
		var feedURL sql.NullString
		var description sql.NullString
		var htmlURL sql.NullString
		var language sql.NullString

		if err := rows.Scan(&title, &feedURL, &description, &htmlURL, &language); err != nil {
			return "", fmt.Errorf("scan podcast row: %w", err)
		}

		if !feedURL.Valid || feedURL.String == "" {
			continue
		}

		outlines = append(outlines, opmlOutline{
			Type:        "rss",
			Text:        title.String,
			Title:       title.String,
			XMLURL:      feedURL.String,
			Description: description.String,
			HTMLURL:     htmlURL.String,
			Language:    language.String,
		})
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("podcast rows error: %w", err)
	}

	opmlData := opml{
		Version: "1.0",
		Head: opmlHead{
			Title: "Audiobookshelf Podcast Subscriptions",
		},
		Body: opmlBody{
			Outlines: outlines,
		},
	}

	xmlBytes, err := xml.MarshalIndent(opmlData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal OPML: %w", err)
	}

	return xml.Header + string(xmlBytes), nil
}

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

// OPML structure
type opml struct {
	XMLName xml.Name `xml:"opml"`
	Version string   `xml:"version,attr"`
	Head    opmlHead `xml:"head"`
	Body    opmlBody `xml:"body"`
}

type opmlHead struct {
	Title string `xml:"title"`
}

type opmlBody struct {
	Outlines []opmlOutline `xml:"outline"`
}

type opmlOutline struct {
	Type        string `xml:"type,attr"`
	Text        string `xml:"text,attr"`
	Title       string `xml:"title,attr"`
	XMLURL      string `xml:"xmlUrl,attr"`
	Description string `xml:"description,attr,omitempty"`
	HTMLURL     string `xml:"htmlUrl,attr,omitempty"`
	Language    string `xml:"language,attr,omitempty"`
}

// Database helper structs
type audioFile struct {
	Index    int     `json:"index"`
	Duration float64 `json:"duration"`
	Codec    string  `json:"codec"`
	MimeType string  `json:"mimeType"`
	Metadata struct {
		Path     string `json:"path"`
		RelPath  string `json:"relPath"`
		Filename string `json:"filename"`
		Ext      string `json:"ext"`
		Size     int64  `json:"size"`
	} `json:"metadata"`
}

type audiobookTrack struct {
	Index       int     `json:"index"`
	Exclude     bool    `json:"exclude"`
	Duration    float64 `json:"duration"`
	Codec       string  `json:"codec"`
	MimeType    string  `json:"mimeType"`
	StartOffset float64 `json:"startOffset"`
	Metadata    struct {
		Path     string `json:"path"`
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
	} `json:"metadata"`
}

type audiobookChapter struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Title string  `json:"title"`
}

type podcastEpData struct {
	ID          string
	Title       string
	AudioFile   string
	PubDate     string
	Description string
	Season      string
	Episode     string
	EpisodeType string
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

// Utility and Helper Functions
func checkUseChapterTitles(tracks []audiobookTrack, chapters []audiobookChapter) bool {
	if len(tracks) != len(chapters) {
		return false
	}
	for i := 0; i < len(tracks); i++ {
		if math.Abs(chapters[i].Start-tracks[i].StartOffset) >= 1.0 {
			return false
		}
	}
	return true
}

func parseTime(s string) time.Time {
	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(ms/1000, (ms%1000)*1000000)
	}
	return time.Time{}
}

func sortPodcastEpisodes(eps []*podcastEpData, descending bool) {
	sort.Slice(eps, func(i, j int) bool {
		tI := parseTime(eps[i].PubDate)
		tJ := parseTime(eps[j].PubDate)
		if descending {
			return tI.After(tJ)
		}
		return tI.Before(tJ)
	})
}

func formatPubDate(dateStr string) string {
	if dateStr == "" {
		return time.Now().UTC().Format(time.RFC1123Z)
	}
	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, dateStr); err == nil {
			return t.UTC().Format(time.RFC1123Z)
		}
	}
	if ms, err := strconv.ParseInt(dateStr, 10, 64); err == nil {
		return time.Unix(ms/1000, (ms%1000)*1000000).UTC().Format(time.RFC1123Z)
	}
	return dateStr
}

func computeMD5(val string) string {
	h := md5.New()
	h.Write([]byte(val))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func hasColumn(ctx context.Context, db *sql.DB, tableName, columnName string) bool {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, dType string
		var notnull int
		var dfltVal sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &dType, &notnull, &dfltVal, &pk); err == nil {
			if name == columnName {
				return true
			}
		}
	}
	if err := rows.Err(); err != nil {
		return false
	}
	return false
}

func queryPodcastEpisode(ctx context.Context, db *sql.DB, epID string) (*podcastEpData, error) {
	hasPubDate := hasColumn(ctx, db, "podcastEpisodes", "pubDate")
	hasDesc := hasColumn(ctx, db, "podcastEpisodes", "description")
	hasSeason := hasColumn(ctx, db, "podcastEpisodes", "season")
	hasEp := hasColumn(ctx, db, "podcastEpisodes", "episode")
	hasEpType := hasColumn(ctx, db, "podcastEpisodes", "episodeType")

	query := "SELECT id, title, audioFile"
	if hasPubDate {
		query += ", pubDate"
	}
	if hasDesc {
		query += ", description"
	}
	if hasSeason {
		query += ", season"
	}
	if hasEp {
		query += ", episode"
	}
	if hasEpType {
		query += ", episodeType"
	}
	query += " FROM podcastEpisodes WHERE id = ?"

	row := db.QueryRowContext(ctx, query, epID)

	var id, title, audioFileStr string
	dest := []interface{}{&id, &title, &audioFileStr}

	var pubDateVal, descVal, seasonVal, epVal, epTypeVal sql.NullString
	if hasPubDate {
		dest = append(dest, &pubDateVal)
	}
	if hasDesc {
		dest = append(dest, &descVal)
	}
	if hasSeason {
		dest = append(dest, &seasonVal)
	}
	if hasEp {
		dest = append(dest, &epVal)
	}
	if hasEpType {
		dest = append(dest, &epTypeVal)
	}

	if err := row.Scan(dest...); err != nil {
		return nil, fmt.Errorf("scan podcast episode: %w", err)
	}

	ep := &podcastEpData{
		ID:        id,
		Title:     title,
		AudioFile: audioFileStr,
	}
	if pubDateVal.Valid {
		ep.PubDate = pubDateVal.String
	}
	if descVal.Valid {
		ep.Description = descVal.String
	}
	if seasonVal.Valid {
		ep.Season = seasonVal.String
	}
	if epVal.Valid {
		ep.Episode = epVal.String
	}
	if epTypeVal.Valid {
		ep.EpisodeType = epTypeVal.String
	}
	return ep, nil
}

func queryPodcastEpisodes(ctx context.Context, db *sql.DB, podcastID string) ([]*podcastEpData, error) {
	hasPubDate := hasColumn(ctx, db, "podcastEpisodes", "pubDate")
	hasDesc := hasColumn(ctx, db, "podcastEpisodes", "description")
	hasSeason := hasColumn(ctx, db, "podcastEpisodes", "season")
	hasEp := hasColumn(ctx, db, "podcastEpisodes", "episode")
	hasEpType := hasColumn(ctx, db, "podcastEpisodes", "episodeType")

	query := "SELECT id, title, audioFile"
	if hasPubDate {
		query += ", pubDate"
	}
	if hasDesc {
		query += ", description"
	}
	if hasSeason {
		query += ", season"
	}
	if hasEp {
		query += ", episode"
	}
	if hasEpType {
		query += ", episodeType"
	}
	query += " FROM podcastEpisodes WHERE podcastId = ?"

	rows, err := db.QueryContext(ctx, query, podcastID)
	if err != nil {
		return nil, fmt.Errorf("query podcast episodes: %w", err)
	}
	defer rows.Close()

	var eps []*podcastEpData
	for rows.Next() {
		var id, title, audioFileStr string
		dest := []interface{}{&id, &title, &audioFileStr}
		var pubDateVal, descVal, seasonVal, epVal, epTypeVal sql.NullString
		if hasPubDate {
			dest = append(dest, &pubDateVal)
		}
		if hasDesc {
			dest = append(dest, &descVal)
		}
		if hasSeason {
			dest = append(dest, &seasonVal)
		}
		if hasEp {
			dest = append(dest, &epVal)
		}
		if hasEpType {
			dest = append(dest, &epTypeVal)
		}

		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("scan podcast episode row: %w", err)
		}

		ep := &podcastEpData{
			ID:        id,
			Title:     title,
			AudioFile: audioFileStr,
		}
		if pubDateVal.Valid {
			ep.PubDate = pubDateVal.String
		}
		if descVal.Valid {
			ep.Description = descVal.String
		}
		if seasonVal.Valid {
			ep.Season = seasonVal.String
		}
		if epVal.Valid {
			ep.Episode = epVal.String
		}
		if epTypeVal.Valid {
			ep.EpisodeType = epTypeVal.String
		}
		eps = append(eps, ep)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("podcast episodes rows error: %w", err)
	}
	return eps, nil
}

func (m *FeedManager) checkUserAccess(ctx context.Context, userID, libraryID string) (bool, error) {
	var userType string
	var isActive int
	var permissionsStr sql.NullString
	err := m.db.QueryRowContext(ctx, "SELECT type, isActive, permissions FROM users WHERE id = ?", userID).Scan(&userType, &isActive, &permissionsStr)
	if err == sql.ErrNoRows {
		return false, fmt.Errorf("user not found: %w", err)
	}
	if err != nil {
		return false, fmt.Errorf("query user permissions: %w", err)
	}
	if isActive == 0 {
		return false, fmt.Errorf("user is inactive")
	}
	if userType == "root" || userType == "admin" {
		return true, nil
	}
	if !permissionsStr.Valid || permissionsStr.String == "" {
		return false, nil
	}

	type userPermissions struct {
		AccessAllLibraries  *bool    `json:"accessAllLibraries"`
		LibrariesAccessible []string `json:"librariesAccessible"`
	}
	var perm userPermissions
	if err := json.Unmarshal([]byte(permissionsStr.String), &perm); err != nil {
		return false, fmt.Errorf("unmarshal user permissions: %w", err)
	}

	if perm.AccessAllLibraries != nil && *perm.AccessAllLibraries {
		return true, nil
	}
	for _, lid := range perm.LibrariesAccessible {
		if lid == libraryID {
			return true, nil
		}
	}
	return false, nil
}
