package feed

import "encoding/xml"

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
