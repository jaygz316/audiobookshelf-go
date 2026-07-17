package db

import (
	"context"
	"database/sql"
)

func GetUserFullByUsername(ctx context.Context, db *sql.DB, username string) (*User, error) {
	row := db.QueryRowContext(ctx, "SELECT id, username, email, pash, type, token, isActive, isLocked, lastSeen, permissions, bookmarks, extraData, createdAt, updatedAt FROM users WHERE username = ?", username)
	return ScanUser(row)
}

func GetUserFullByID(ctx context.Context, db *sql.DB, id string) (*User, error) {
	row := db.QueryRowContext(ctx, "SELECT id, username, email, pash, type, token, isActive, isLocked, lastSeen, permissions, bookmarks, extraData, createdAt, updatedAt FROM users WHERE id = ?", id)
	return ScanUser(row)
}

func CheckUserExistsWithUsername(ctx context.Context, db *sql.DB, username string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE username = ?", username).Scan(&count)
	return count > 0, err
}

func ScanUser(row *sql.Row) (*User, error) {
	var u User
	var email sql.NullString
	var lastSeenStr sql.NullString
	var isActiveInt, isLockedInt sql.NullInt64
	var permsStr, bookmarksStr, extraDataStr sql.NullString
	var createdAtStr, updatedAtStr sql.NullString
	var pashStr, tokenStr, typeStr sql.NullString

	err := row.Scan(&u.ID, &u.Username, &email, &pashStr, &typeStr, &tokenStr, &isActiveInt, &isLockedInt, &lastSeenStr, &permsStr, &bookmarksStr, &extraDataStr, &createdAtStr, &updatedAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if typeStr.Valid {
		u.Type = typeStr.String
	} else {
		u.Type = "user"
	}

	if pashStr.Valid {
		u.Pash = pashStr.String
	}
	if tokenStr.Valid {
		u.Token = tokenStr.String
	}
	if email.Valid {
		u.Email = &email.String
	}
	u.IsActive = isActiveInt.Valid && isActiveInt.Int64 != 0
	u.IsLocked = isLockedInt.Valid && isLockedInt.Int64 != 0
	if lastSeenStr.Valid && lastSeenStr.String != "" {
		val := ParseTimeStr(lastSeenStr.String)
		u.LastSeen = &val
	}

	if permsStr.Valid {
		u.Permissions = []byte(permsStr.String)
	} else {
		u.Permissions = []byte("{}")
	}

	if bookmarksStr.Valid {
		u.Bookmarks = []byte(bookmarksStr.String)
	} else {
		u.Bookmarks = []byte("[]")
	}

	if extraDataStr.Valid {
		u.ExtraData = []byte(extraDataStr.String)
	} else {
		u.ExtraData = []byte("{}")
	}

	u.CreatedAt = ParseTimeStr(createdAtStr.String)
	u.UpdatedAt = ParseTimeStr(updatedAtStr.String)

	return &u, nil
}
