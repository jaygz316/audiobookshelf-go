package podcast

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
)

func latin1ToUTF8(latin1Str string) string {
	runes := make([]rune, len(latin1Str))
	for i, b := range []byte(latin1Str) {
		runes[i] = rune(b)
	}
	return string(runes)
}

func parseRSS(xmlData []byte) (*PodcastFeed, error) {
	decoder := xml.NewDecoder(bytes.NewReader(xmlData))
	decoder.CharsetReader = charset.NewReaderLabel
	decoder.Entity = xml.HTMLEntity

	var feed PodcastFeed
	var episodes []*PodcastEpisode

	var currentEp *PodcastEpisode
	var elementStack []string

	var channelTitle, channelAuthor, channelDescription, channelITunesSummary strings.Builder
	var itemTitle, itemDescription, itemContentEncoded, itemPubDate, itemDuration, itemITunesDuration strings.Builder
	var itemSeason, itemEpisode, itemEpisodeType strings.Builder
	var itemEnclosureURL string
	var itemImageURL string

	for {
		t, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("xml token: %w", err)
		}

		switch se := t.(type) {
		case xml.StartElement:
			elementStack = append(elementStack, se.Name.Local)

			if se.Name.Local == "item" {
				currentEp = &PodcastEpisode{}
				itemTitle.Reset()
				itemDescription.Reset()
				itemContentEncoded.Reset()
				itemPubDate.Reset()
				itemDuration.Reset()
				itemITunesDuration.Reset()
				itemEnclosureURL = ""
				itemSeason.Reset()
				itemEpisode.Reset()
				itemEpisodeType.Reset()
				itemImageURL = ""
			}

			if currentEp != nil && len(elementStack) >= 4 && elementStack[len(elementStack)-2] == "item" {
				localName := se.Name.Local
				if localName == "enclosure" {
					for _, attr := range se.Attr {
						if attr.Name.Local == "url" {
							itemEnclosureURL = attr.Value
						}
					}
				} else if localName == "content" {
					isAudio := false
					var urlVal string
					for _, attr := range se.Attr {
						if attr.Name.Local == "type" && strings.HasPrefix(attr.Value, "audio") {
							isAudio = true
						}
						if attr.Name.Local == "url" {
							urlVal = attr.Value
						}
					}
					if isAudio && urlVal != "" && itemEnclosureURL == "" {
						itemEnclosureURL = urlVal
					}
				} else if localName == "image" {
					for _, attr := range se.Attr {
						if attr.Name.Local == "href" {
							itemImageURL = attr.Value
						}
					}
				}
			}

		case xml.EndElement:
			if se.Name.Local == "item" && currentEp != nil {
				desc := itemContentEncoded.String()
				if desc == "" {
					desc = itemDescription.String()
				}

				durStr := itemITunesDuration.String()
				if durStr == "" {
					durStr = itemDuration.String()
				}
				durSec := parseDurationToSeconds(durStr)

				currentEp.Title = strings.TrimSpace(itemTitle.String())
				currentEp.Description = strings.TrimSpace(desc)
				currentEp.EnclosureURL = strings.TrimSpace(itemEnclosureURL)
				currentEp.PublishedAt = strings.TrimSpace(itemPubDate.String())
				currentEp.Duration = durSec
				currentEp.Season = strings.TrimSpace(itemSeason.String())
				currentEp.Episode = strings.TrimSpace(itemEpisode.String())
				currentEp.EpisodeType = strings.TrimSpace(itemEpisodeType.String())
				currentEp.ImageURL = strings.TrimSpace(itemImageURL)

				episodes = append(episodes, currentEp)
				currentEp = nil
			}

			if len(elementStack) > 0 {
				elementStack = elementStack[:len(elementStack)-1]
			}

		case xml.CharData:
			if len(elementStack) < 3 {
				continue
			}
			val := string(se)
			parent := elementStack[len(elementStack)-1]
			grandParent := elementStack[len(elementStack)-2]

			if grandParent == "item" && currentEp != nil {
				switch parent {
				case "title":
					itemTitle.WriteString(val)
				case "description":
					itemDescription.WriteString(val)
				case "encoded":
					itemContentEncoded.WriteString(val)
				case "pubDate":
					itemPubDate.WriteString(val)
				case "duration":
					itemITunesDuration.WriteString(val)
				case "season":
					itemSeason.WriteString(val)
				case "episode":
					itemEpisode.WriteString(val)
				case "episodeType":
					itemEpisodeType.WriteString(val)
				}
			} else if grandParent == "channel" {
				switch parent {
				case "title":
					channelTitle.WriteString(val)
				case "author":
					channelAuthor.WriteString(val)
				case "description":
					channelDescription.WriteString(val)
				case "summary":
					channelITunesSummary.WriteString(val)
				}
			}
		}
	}

	feed.Title = strings.TrimSpace(channelTitle.String())
	feed.Author = strings.TrimSpace(channelAuthor.String())

	desc := strings.TrimSpace(channelDescription.String())
	if desc == "" {
		desc = strings.TrimSpace(channelITunesSummary.String())
	}
	feed.Description = desc

	for _, ep := range episodes {
		if ep.PublishedAt != "" {
			t := parseTime(ep.PublishedAt)
			if !t.IsZero() {
				ep.PublishedAt = t.UTC().Format(time.RFC3339)
			}
		}
	}

	feed.Episodes = episodes
	return &feed, nil
}
