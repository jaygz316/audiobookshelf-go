package feed

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"
)

// buildBookItems converts audiobook tracks into feed items.
func buildBookItems(
	tracks []audiobookTrack,
	chapters []audiobookChapter,
	liCreatedAt time.Time,
	title string,
	desc string,
	explicit int,
	hostPrefix string,
	feedBaseURL string,
	contextID string,
	mediaItemID string,
	isDirectBook bool,
	sequence string,
) []item {
	var items []item
	useChapterTitles := checkUseChapterTitles(tracks, chapters)

	for i, t := range tracks {
		if t.Exclude {
			continue
		}
		var trackID string
		var itemURL string
		if isDirectBook {
			trackID = computeMD5(t.Metadata.Path)
			itemURL = hostPrefix + "/item/" + contextID
		} else {
			trackID = computeMD5(contextID + "_" + mediaItemID + "_" + t.Metadata.Path)
			itemURL = hostPrefix + "/item/" + mediaItemID
		}
		ext := filepath.Ext(t.Metadata.Filename)

		epTitle := strings.TrimSuffix(t.Metadata.Filename, ext)
		if len(tracks) == 1 {
			epTitle = title
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
			Description: desc,
			URL:         itemURL,
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
		if desc != "" {
			itemVal.ITunesSummary = &cdata{Value: desc}
		}
		items = append(items, itemVal)
	}
	return items
}

// buildPodcastEpisodeItem converts a single podcast episode into a feed item.
func buildPodcastEpisodeItem(
	ep *podcastEpData,
	author string,
	explicit int,
	hostPrefix string,
	feedBaseURL string,
) item {
	var af audioFile
	_ = json.Unmarshal([]byte(ep.AudioFile), &af)

	itemVal := item{
		Title:        ep.Title,
		Description:  ep.Description,
		URL:          hostPrefix + "/item/" + ep.ID,
		GUID:         feedBaseURL + "/item/" + ep.ID + "/media",
		Author:       author,
		ITunesAuthor: author,
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
	return itemVal
}
