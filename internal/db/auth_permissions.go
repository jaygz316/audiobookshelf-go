package db

import (
	"database/sql"
	"encoding/json"

	"audiobookshelf/internal/core"
)

// userPermissions is the internal struct for parsing DB permissions JSON.
type userPermissions struct {
	Download                  *bool    `json:"download"`
	Upload                    *bool    `json:"upload"`
	Delete                    *bool    `json:"delete"`
	Update                    *bool    `json:"update"`
	AccessRss                 *bool    `json:"accessRss"`
	CreatePublicShares        *bool    `json:"createShares"`
	AccessExplicitContent     *bool    `json:"accessExplicitContent"`
	AccessAllLibraries        *bool    `json:"accessAllLibraries"`
	LibrariesAccessible       []string `json:"librariesAccessible"`
	Libraries                 []string `json:"libraries"`
	AccessAllTags             *bool    `json:"accessAllTags"`
	ItemTagsSelected          []string `json:"itemTagsSelected"`
	SelectedTagsNotAccessible *bool    `json:"selectedTagsNotAccessible"`
}

// ParsePermissions parses the permissions JSON string into a UserSession.
func ParsePermissions(permsStr sql.NullString, user *core.UserSession) {
	// default values:
	user.CanDownload = true
	user.CanUpload = true
	user.CanDelete = false
	user.CanUpdate = false
	user.CanAccessRss = true
	user.CanCreateShares = true
	user.CanAccessExplicitContent = false
	user.AccessAllLibraries = true
	user.LibrariesAccessible = []string{}
	user.AccessAllTags = true
	user.ItemTagsSelected = []string{}
	user.SelectedTagsNotAccessible = false

	// if it's admin or root, they have all access by default
	if user.Type == "root" || user.Type == "admin" {
		user.CanAccessExplicitContent = true
		user.AccessAllLibraries = true
		user.AccessAllTags = true
		user.CanUpload = true
		user.CanDelete = true
		user.CanUpdate = true
		user.CanAccessRss = true
		user.CanCreateShares = true
	}

	if !permsStr.Valid || permsStr.String == "" {
		return
	}

	var perms userPermissions
	if err := json.Unmarshal([]byte(permsStr.String), &perms); err != nil {
		return
	}

	if perms.Download != nil {
		user.CanDownload = *perms.Download
	}
	if perms.Upload != nil {
		user.CanUpload = *perms.Upload
	}
	if perms.Delete != nil {
		user.CanDelete = *perms.Delete
	}
	if perms.Update != nil {
		user.CanUpdate = *perms.Update
	}
	if perms.AccessRss != nil {
		user.CanAccessRss = *perms.AccessRss
	}
	if perms.CreatePublicShares != nil {
		user.CanCreateShares = *perms.CreatePublicShares
	}
	if perms.AccessExplicitContent != nil {
		user.CanAccessExplicitContent = *perms.AccessExplicitContent
	}
	if perms.AccessAllLibraries != nil {
		user.AccessAllLibraries = *perms.AccessAllLibraries
	}
	if perms.LibrariesAccessible != nil {
		user.LibrariesAccessible = perms.LibrariesAccessible
		if perms.AccessAllLibraries == nil {
			user.AccessAllLibraries = false
		}
	} else if perms.Libraries != nil {
		user.LibrariesAccessible = perms.Libraries
		if perms.AccessAllLibraries == nil {
			user.AccessAllLibraries = false
		}
	}
	if perms.AccessAllTags != nil {
		user.AccessAllTags = *perms.AccessAllTags
	}
	if perms.ItemTagsSelected != nil {
		user.ItemTagsSelected = perms.ItemTagsSelected
	}
	if perms.SelectedTagsNotAccessible != nil {
		user.SelectedTagsNotAccessible = *perms.SelectedTagsNotAccessible
	}
}
