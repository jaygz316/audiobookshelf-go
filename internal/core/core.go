// Package core contains shared types used across the audiobookshelf application.
package core

import (
	"github.com/golang-jwt/jwt/v5"
)

// ContextKey is the type used for context keys to avoid collisions.
type ContextKey string

// UserContextKey is the context key for the authenticated user session.
const UserContextKey ContextKey = "user"

// UserSession holds the minimum user info needed to process a request.
// It is loaded once per request by the auth middleware and injected into
// the request context.
type UserSession struct {
	ID                        string
	Username                  string
	Type                      string
	IsActive                  bool
	CanDownload               bool
	CanAccessExplicitContent  bool
	AccessAllLibraries        bool
	LibrariesAccessible       []string
	AccessAllTags             bool
	ItemTagsSelected          []string
	SelectedTagsNotAccessible bool
}

// CanAccessLibrary returns true if the user may access the given library.
func (u *UserSession) CanAccessLibrary(libraryID string) bool {
	if u.Type == "root" || u.Type == "admin" {
		return true
	}
	if u.AccessAllLibraries {
		return true
	}
	for _, id := range u.LibrariesAccessible {
		if id == libraryID {
			return true
		}
	}
	return false
}

// IsAdminOrUp returns true if the user is an admin or root.
func (u *UserSession) IsAdminOrUp() bool {
	return u.Type == "root" || u.Type == "admin"
}

// CheckCanAccessLibraryItem checks if a user session has permissions to view a library item
// represented as a map (used for socket broadcast filtering).
func (u *UserSession) CheckCanAccessLibraryItem(item map[string]interface{}) bool {
	libID, _ := item["libraryId"].(string)
	if libID == "" {
		if media, ok := item["media"].(map[string]interface{}); ok {
			libID, _ = media["libraryId"].(string)
		}
	}
	if !u.CanAccessLibrary(libID) {
		return false
	}

	var isExplicit bool
	if media, ok := item["media"].(map[string]interface{}); ok {
		if exp, ok := media["explicit"].(bool); ok && exp {
			isExplicit = true
		}
		if metadata, ok := media["metadata"].(map[string]interface{}); ok {
			if exp, ok := metadata["explicit"].(bool); ok && exp {
				isExplicit = true
			}
		}
	}
	if isExplicit && !u.CanAccessExplicitContent {
		return false
	}

	var tags []string
	if media, ok := item["media"].(map[string]interface{}); ok {
		if rawTags, ok := media["tags"].([]interface{}); ok {
			for _, t := range rawTags {
				if ts, ok := t.(string); ok {
					tags = append(tags, ts)
				}
			}
		}
	}
	return u.CheckCanAccessLibraryItemWithTags(tags)
}

// CheckCanAccessLibraryItemWithTags validates tag filters for a library item.
func (u *UserSession) CheckCanAccessLibraryItemWithTags(tags []string) bool {
	if u.AccessAllTags {
		return true
	}

	selectedTags := make(map[string]bool)
	for _, t := range u.ItemTagsSelected {
		selectedTags[t] = true
	}

	if u.SelectedTagsNotAccessible {
		if len(tags) == 0 {
			return true
		}
		for _, t := range tags {
			if selectedTags[t] {
				return false
			}
		}
		return true
	}

	if len(tags) == 0 {
		return false
	}
	for _, t := range tags {
		if selectedTags[t] {
			return true
		}
	}
	return false
}

// AuthClaims represents the structure of Audiobookshelf JWT claims.
type AuthClaims struct {
	UserID   string `json:"userId,omitempty"`
	Username string `json:"username,omitempty"`
	KeyID    string `json:"keyId,omitempty"`
	Type     string `json:"type,omitempty"`
	jwt.RegisteredClaims
}

// LogMessage represents a single log entry formatted for the client.
type LogMessage struct {
	Timestamp string `json:"timestamp"`
	Level     int    `json:"level"`
	LevelName string `json:"levelName"`
	Message   string `json:"message"`
}
