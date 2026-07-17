package db

import (
	"encoding/json"
)

// User represents the full user structure matching the SQLite table
type User struct {
	ID          string          `json:"id"`
	Username    string          `json:"username"`
	Email       *string         `json:"email"`
	Pash        string          `json:"-"`
	Type        string          `json:"type"`
	Token       string          `json:"token"`
	IsActive    bool            `json:"isActive"`
	IsLocked    bool            `json:"isLocked"`
	LastSeen    *int64          `json:"lastSeen"`
	Permissions json.RawMessage `json:"permissions"`
	Bookmarks   json.RawMessage `json:"bookmarks"`
	ExtraData   json.RawMessage `json:"extraData"`
	CreatedAt   int64           `json:"createdAt"`
	UpdatedAt   int64           `json:"updatedAt"`
}

// UserPermissionsDetailed corresponds to the parsed permissions object stored in the DB
type UserPermissionsDetailed struct {
	Download                  *bool    `json:"download,omitempty"`
	Upload                    *bool    `json:"upload,omitempty"`
	Delete                    *bool    `json:"delete,omitempty"`
	Update                    *bool    `json:"update,omitempty"`
	AccessRss                 *bool    `json:"accessRss,omitempty"`
	CreatePublicShares        *bool    `json:"createShares,omitempty"`
	AccessExplicitContent     *bool    `json:"accessExplicitContent,omitempty"`
	AccessAllLibraries        *bool    `json:"accessAllLibraries,omitempty"`
	LibrariesAccessible       []string `json:"librariesAccessible,omitempty"`
	AccessAllTags             *bool    `json:"accessAllTags,omitempty"`
	ItemTagsSelected          []string `json:"itemTagsSelected,omitempty"`
	SelectedTagsNotAccessible *bool    `json:"selectedTagsNotAccessible,omitempty"`
}

// UserSessionDB matches the sessions table
type UserSessionDB struct {
	ID                        string `db:"id"`
	IPAddress                 string `db:"ipAddress"`
	UserAgent                 string `db:"userAgent"`
	RefreshToken              string `db:"refreshToken"`
	ExpiresAt                 int64  `db:"expiresAt"`
	LastRefreshToken          string `db:"lastRefreshToken"`
	LastRefreshTokenExpiresAt int64  `db:"lastRefreshTokenExpiresAt"`
	UserID                    string `db:"userId"`
	CreatedAt                 int64  `db:"createdAt"`
	UpdatedAt                 int64  `db:"updatedAt"`
}

// ToOldJSONForBrowser maps User to the format client expects
func (u *User) ToOldJSONForBrowser(hideRootToken bool) map[string]interface{} {
	var perms map[string]interface{}
	if len(u.Permissions) > 0 {
		json.Unmarshal(u.Permissions, &perms)
	}
	if perms == nil {
		perms = make(map[string]interface{})
	}

	librariesAccessible := []string{}
	if libs, ok := perms["librariesAccessible"]; ok {
		if libsArr, ok2 := libs.([]interface{}); ok2 {
			for _, libVal := range libsArr {
				if libStr, ok3 := libVal.(string); ok3 {
					librariesAccessible = append(librariesAccessible, libStr)
				}
			}
		}
	}
	itemTagsSelected := []string{}
	if tags, ok := perms["itemTagsSelected"]; ok {
		if tagsArr, ok2 := tags.([]interface{}); ok2 {
			for _, tagVal := range tagsArr {
				if tagStr, ok3 := tagVal.(string); ok3 {
					itemTagsSelected = append(itemTagsSelected, tagStr)
				}
			}
		}
	}

	delete(perms, "librariesAccessible")
	delete(perms, "itemTagsSelected")

	var extra map[string]interface{}
	if len(u.ExtraData) > 0 {
		json.Unmarshal(u.ExtraData, &extra)
	}
	if extra == nil {
		extra = make(map[string]interface{})
	}

	seriesHideFromContinueListening := []string{}
	if hfc, ok := extra["seriesHideFromContinueListening"]; ok {
		if hfcArr, ok2 := hfc.([]interface{}); ok2 {
			for _, hVal := range hfcArr {
				if hStr, ok3 := hVal.(string); ok3 {
					seriesHideFromContinueListening = append(seriesHideFromContinueListening, hStr)
				}
			}
		}
	}

	var bookmarksArr []interface{}
	if len(u.Bookmarks) > 0 {
		json.Unmarshal(u.Bookmarks, &bookmarksArr)
	}
	if bookmarksArr == nil {
		bookmarksArr = []interface{}{}
	}

	token := u.Token
	if u.Type == "root" && hideRootToken {
		token = ""
	}

	hasOpenIDLink := false
	if oSub, ok := extra["authOpenIDSub"]; ok && oSub != nil && oSub != "" {
		hasOpenIDLink = true
	}

	return map[string]interface{}{
		"id":                              u.ID,
		"username":                        u.Username,
		"email":                           u.Email,
		"type":                            u.Type,
		"token":                           token,
		"isOldToken":                      false,
		"mediaProgress":                   []interface{}{}, // Loaded separately if requested
		"seriesHideFromContinueListening": seriesHideFromContinueListening,
		"bookmarks":                       bookmarksArr,
		"isActive":                        u.IsActive,
		"isLocked":                        u.IsLocked,
		"lastSeen":                        u.LastSeen,
		"createdAt":                       u.CreatedAt,
		"permissions":                     perms,
		"librariesAccessible":             librariesAccessible,
		"itemTagsSelected":                itemTagsSelected,
		"hasOpenIDLink":                   hasOpenIDLink,
	}
}
