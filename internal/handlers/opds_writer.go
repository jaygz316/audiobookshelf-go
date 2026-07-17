package handlers

import (
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

// writeItemEntries formats each minified library item as an Atom XML entry.
func writeItemEntries(sb *strings.Builder, items []*idb.LibraryItemMinifiedJSON, r *http.Request) {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if xfh := r.Header.Get("X-Forwarded-Host"); xfh != "" {
		host = xfh
	}

	baseURL := fmt.Sprintf("%s://%s", scheme, host)

	for _, item := range items {
		title := ""
		author := ""
		narrator := ""
		description := ""
		mimeType := "application/zip" // Default for audiobook download folders

		if item.MediaType == "book" {
			if book, ok := item.Media.(*idb.BookMinifiedJSON); ok && book != nil {
				title = book.Metadata.Title
				if book.Metadata.Subtitle != nil {
					title = fmt.Sprintf("%s - %s", title, *book.Metadata.Subtitle)
				}
				author = book.Metadata.AuthorName
				narrator = book.Metadata.NarratorName
				if book.Metadata.Description != nil {
					description = *book.Metadata.Description
				}
				if book.EbookFormat != nil && *book.EbookFormat != "" {
					fmtStr := strings.ToLower(*book.EbookFormat)
					if fmtStr == "epub" {
						mimeType = "application/epub+zip"
					} else if fmtStr == "pdf" {
						mimeType = "application/pdf"
					} else if fmtStr == "mobi" {
						mimeType = "application/x-mobipocket-ebook"
					}
				}
			}
		} else if item.MediaType == "podcast" {
			if podcast, ok := item.Media.(*idb.PodcastMinifiedJSON); ok && podcast != nil {
				title = podcast.Metadata.Title
				if podcast.Metadata.Author != nil {
					author = *podcast.Metadata.Author
				}
				if podcast.Metadata.Description != nil {
					description = *podcast.Metadata.Description
				}
			}
		}

		updatedTime := time.Unix(0, item.UpdatedAt*int64(time.Millisecond)).UTC().Format(time.RFC3339)

		sb.WriteString("  <entry>\n")
		sb.WriteString(fmt.Sprintf("    <title>%s</title>\n", html.EscapeString(title)))
		sb.WriteString(fmt.Sprintf("    <id>urn:uuid:%s</id>\n", item.ID))
		sb.WriteString(fmt.Sprintf("    <updated>%s</updated>\n", updatedTime))

		if author != "" {
			sb.WriteString(fmt.Sprintf("    <author><name>%s</name></author>\n", html.EscapeString(author)))
		}
		if narrator != "" {
			sb.WriteString(fmt.Sprintf("    <contributor role=\"nrt\"><name>%s</name></contributor>\n", html.EscapeString(narrator)))
		}

		if description != "" {
			sb.WriteString(fmt.Sprintf("    <content type=\"text\">%s</content>\n", html.EscapeString(description)))
		} else {
			sb.WriteString("    <content type=\"text\">No description available.</content>\n")
		}

		// Covers
		sb.WriteString(fmt.Sprintf("    <link rel=\"http://opds-spec.org/image\" href=\"%s/api/items/%s/cover\" type=\"image/jpeg\"/>\n", baseURL, item.ID))
		sb.WriteString(fmt.Sprintf("    <link rel=\"http://opds-spec.org/image/thumbnail\" href=\"%s/api/items/%s/cover?width=200\" type=\"image/jpeg\"/>\n", baseURL, item.ID))

		// Acquisition/Download Link
		sb.WriteString(fmt.Sprintf("    <link rel=\"http://opds-spec.org/acquisition\" href=\"%s/api/items/%s/download\" type=\"%s\"/>\n", baseURL, item.ID, mimeType))

		sb.WriteString("  </entry>\n")
	}
}

// canUserAccessItemMinified checks if the user has access to the minified library item based on library permissions, tags, and explicit content restriction.
func canUserAccessItemMinified(user *core.UserSession, item *idb.LibraryItemMinifiedJSON) bool {
	if user == nil {
		return false
	}
	if !user.CanAccessLibrary(item.LibraryID) {
		return false
	}

	var isExplicit bool
	var tags []string

	if item.MediaType == "book" {
		if book, ok := item.Media.(*idb.BookMinifiedJSON); ok && book != nil {
			tags = book.Tags
			if book.Metadata != nil {
				isExplicit = book.Metadata.Explicit
			}
		} else if book, ok := item.Media.(idb.BookMinifiedJSON); ok {
			tags = book.Tags
			if book.Metadata != nil {
				isExplicit = book.Metadata.Explicit
			}
		}
	} else if item.MediaType == "podcast" {
		if podcast, ok := item.Media.(*idb.PodcastMinifiedJSON); ok && podcast != nil {
			tags = podcast.Tags
			if podcast.Metadata != nil {
				isExplicit = podcast.Metadata.Explicit
			}
		} else if podcast, ok := item.Media.(idb.PodcastMinifiedJSON); ok {
			tags = podcast.Tags
			if podcast.Metadata != nil {
				isExplicit = podcast.Metadata.Explicit
			}
		}
	}

	if isExplicit && !user.CanAccessExplicitContent {
		return false
	}

	return user.CheckCanAccessLibraryItemWithTags(tags)
}
