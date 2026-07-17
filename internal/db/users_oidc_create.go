package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"audiobookshelf/internal/core"
)

func CreateUserFromOpenIdUserInfo(ctx context.Context, db *sql.DB, userinfo map[string]interface{}, tokenSecret string, userType string) (*User, error) {
	userId := uuid.New().String()
	username, _ := userinfo["preferred_username"].(string)
	if username == "" {
		username, _ = userinfo["name"].(string)
	}
	if username == "" {
		username, _ = userinfo["sub"].(string)
	}

	var emailVal *string
	if email, ok := userinfo["email"].(string); ok && email != "" {
		if ev, ok2 := userinfo["email_verified"].(bool); !ok2 || ev {
			emailVal = &email
		}
	}

	claims := &core.AuthClaims{
		UserID:   userId,
		Username: username,
		Type:     userType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "audiobookshelf",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(tokenSecret))
	if err != nil {
		return nil, err
	}

	extra := map[string]interface{}{
		"authOpenIDSub":                   userinfo["sub"],
		"seriesHideFromContinueListening": []string{},
	}
	extraBytes, _ := json.Marshal(extra)
	if userType == "" {
		userType = "user"
	}
	perms := GetDefaultPermissionsForUserType(userType)
	nowStr := TimeToDBStr(time.Now())

	var emailStr sql.NullString
	if emailVal != nil {
		emailStr.String = *emailVal
		emailStr.Valid = true
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO users (id, username, email, pash, type, token, isActive, isLocked, permissions, bookmarks, extraData, createdAt, updatedAt)
		VALUES (?, ?, ?, NULL, ?, ?, 1, 0, ?, '[]', ?, ?, ?)`,
		userId, username, emailStr, userType, tokenString, perms, string(extraBytes), nowStr, nowStr)
	if err != nil {
		return nil, err
	}

	return &User{
		ID:          userId,
		Username:    username,
		Email:       emailVal,
		Type:        userType,
		Token:       tokenString,
		IsActive:    true,
		IsLocked:    false,
		Permissions: []byte(perms),
		Bookmarks:   []byte("[]"),
		ExtraData:   extraBytes,
		CreatedAt:   time.Now().UnixNano() / int64(time.Millisecond),
		UpdatedAt:   time.Now().UnixNano() / int64(time.Millisecond),
	}, nil
}

func UpdateUserTypeAndToken(ctx context.Context, db *sql.DB, u *User, newType string, tokenSecret string) error {
	claims := &core.AuthClaims{
		UserID:   u.ID,
		Username: u.Username,
		Type:     newType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "audiobookshelf",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(tokenSecret))
	if err != nil {
		return err
	}

	perms := GetDefaultPermissionsForUserType(newType)
	_, err = db.ExecContext(ctx, "UPDATE users SET type = ?, token = ?, permissions = ?, updatedAt = ? WHERE id = ?",
		newType, tokenString, perms, TimeToDBStr(time.Now()), u.ID)
	if err != nil {
		return err
	}

	u.Type = newType
	u.Token = tokenString
	u.Permissions = []byte(perms)
	return nil
}
