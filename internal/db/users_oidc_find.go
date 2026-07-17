package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	log "audiobookshelf/internal/logger"
)

func FindUserFromOpenIdUserInfo(ctx context.Context, db *sql.DB, userinfo map[string]interface{}, matchBy string) (*User, error) {
	sub, _ := userinfo["sub"].(string)
	if sub == "" {
		return nil, fmt.Errorf("invalid userinfo, no sub")
	}

	var u *User
	row := db.QueryRowContext(ctx, "SELECT id, username, email, pash, type, token, isActive, isLocked, lastSeen, permissions, bookmarks, extraData, createdAt, updatedAt FROM users WHERE json_extract(extraData, '$.authOpenIDSub') = ?", sub)
	u, err := ScanUser(row)
	if err == nil && u != nil {
		log.Printf("[User] openid: User found by sub %s", sub)
		return u, nil
	}

	if matchBy == "email" {
		email, _ := userinfo["email"].(string)
		if email != "" {
			if verified, ok := userinfo["email_verified"].(bool); ok && !verified {
				return nil, fmt.Errorf("email not verified")
			}
			row = db.QueryRowContext(ctx, "SELECT id, username, email, pash, type, token, isActive, isLocked, lastSeen, permissions, bookmarks, extraData, createdAt, updatedAt FROM users WHERE lower(email) = ?", strings.ToLower(email))
			u, err = ScanUser(row)
			if err == nil && u != nil {
				var extra map[string]interface{}
				json.Unmarshal(u.ExtraData, &extra)
				if oSub, ok := extra["authOpenIDSub"]; ok && oSub != nil && oSub != "" && oSub != sub {
					return nil, fmt.Errorf("user already linked to a different OpenID subject")
				}
				if extra == nil {
					extra = make(map[string]interface{})
				}
				extra["authOpenIDSub"] = sub
				extraBytes, _ := json.Marshal(extra)
				_, err = db.ExecContext(ctx, "UPDATE users SET extraData = ? WHERE id = ?", string(extraBytes), u.ID)
				if err != nil {
					return nil, err
				}
				u.ExtraData = extraBytes
				return u, nil
			}
		} else {
			return nil, fmt.Errorf("no email in userinfo")
		}
	} else if matchBy == "username" {
		var username string
		if pu, ok := userinfo["preferred_username"].(string); ok && pu != "" {
			username = pu
		} else if un, ok := userinfo["username"].(string); ok && un != "" {
			username = un
		} else if name, ok := userinfo["name"].(string); ok && name != "" {
			username = name
		}
		if username != "" {
			row = db.QueryRowContext(ctx, "SELECT id, username, email, pash, type, token, isActive, isLocked, lastSeen, permissions, bookmarks, extraData, createdAt, updatedAt FROM users WHERE lower(username) = ?", strings.ToLower(username))
			u, err = ScanUser(row)
			if err == nil && u != nil {
				var extra map[string]interface{}
				json.Unmarshal(u.ExtraData, &extra)
				if oSub, ok := extra["authOpenIDSub"]; ok && oSub != nil && oSub != "" && oSub != sub {
					return nil, fmt.Errorf("user already linked to a different OpenID subject")
				}
				if extra == nil {
					extra = make(map[string]interface{})
				}
				extra["authOpenIDSub"] = sub
				extraBytes, _ := json.Marshal(extra)
				_, err = db.ExecContext(ctx, "UPDATE users SET extraData = ? WHERE id = ?", string(extraBytes), u.ID)
				if err != nil {
					return nil, err
				}
				u.ExtraData = extraBytes
				return u, nil
			}
		} else {
			return nil, fmt.Errorf("no username in userinfo")
		}
	}

	return nil, nil
}
