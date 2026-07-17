package feed

import (
	"encoding/xml"
	"net/http"
	"strings"
)

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

	var err error
	switch entityType {
	case "playlist":
		err = m.buildPlaylistChannel(ctx, itemID, hostPrefix, feedBaseURL, &feedChannel)
	case "collection":
		err = m.buildCollectionChannel(ctx, itemID, hostPrefix, feedBaseURL, &feedChannel)
	case "series":
		err = m.buildSeriesChannel(ctx, itemID, hostPrefix, feedBaseURL, &feedChannel)
	default:
		err = m.buildLibraryItemChannel(ctx, itemID, hostPrefix, feedBaseURL, &feedChannel)
	}

	if err != nil {
		http.NotFound(w, r)
		return
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
