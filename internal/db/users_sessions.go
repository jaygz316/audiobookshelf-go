package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

func CreateSession(ctx context.Context, db *sql.DB, userID, ipAddress, userAgent, refreshToken string, expiresAt time.Time) error {
	sessionID := uuid.New().String()
	nowStr := TimeToDBStr(time.Now())
	expiresStr := TimeToDBStr(expiresAt)
	_, err := db.ExecContext(ctx, `INSERT INTO sessions (id, userId, ipAddress, userAgent, refreshToken, expiresAt, lastRefreshToken, lastRefreshTokenExpiresAt, createdAt, updatedAt) 
		VALUES (?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?)`,
		sessionID, userID, ipAddress, userAgent, refreshToken, expiresStr, nowStr, nowStr)
	return err
}

func DeleteSessionByRefreshToken(ctx context.Context, db *sql.DB, refreshToken string) (int64, error) {
	res, err := db.ExecContext(ctx, "DELETE FROM sessions WHERE refreshToken = ?", refreshToken)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func CleanupExpiredSessions(ctx context.Context, db *sql.DB) (int64, error) {
	nowStr := TimeToDBStr(time.Now())
	res, err := db.ExecContext(ctx, "DELETE FROM sessions WHERE expiresAt < ?", nowStr)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// GetUserSessions retrieves all active login sessions for the given user ID.
func GetUserSessions(ctx context.Context, db *sql.DB, userID string) ([]UserSessionDB, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, userId, ipAddress, userAgent, refreshToken, expiresAt, lastRefreshToken, lastRefreshTokenExpiresAt, createdAt, updatedAt FROM sessions WHERE userId = ? ORDER BY COALESCE(updatedAt, createdAt) DESC", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []UserSessionDB
	for rows.Next() {
		var s UserSessionDB
		var expiresAtStr string
		var lastExpiresAtStr sql.NullString
		var lastRefreshToken sql.NullString
		var createdAtStr, updatedAtStr sql.NullString

		err := rows.Scan(&s.ID, &s.UserID, &s.IPAddress, &s.UserAgent, &s.RefreshToken, &expiresAtStr, &lastRefreshToken, &lastExpiresAtStr, &createdAtStr, &updatedAtStr)
		if err != nil {
			return nil, err
		}

		s.ExpiresAt = ParseTimeStr(expiresAtStr)
		if lastRefreshToken.Valid {
			s.LastRefreshToken = lastRefreshToken.String
		}
		if lastExpiresAtStr.Valid {
			s.LastRefreshTokenExpiresAt = ParseTimeStr(lastExpiresAtStr.String)
		}
		if createdAtStr.Valid {
			s.CreatedAt = ParseTimeStr(createdAtStr.String)
		}
		if updatedAtStr.Valid {
			s.UpdatedAt = ParseTimeStr(updatedAtStr.String)
		}

		sessions = append(sessions, s)
	}

	if sessions == nil {
		sessions = []UserSessionDB{}
	}
	return sessions, nil
}

// DeleteSessionByID deletes a login session from the sessions table by ID.
func DeleteSessionByID(ctx context.Context, db *sql.DB, sessionID string) error {
	_, err := db.ExecContext(ctx, "DELETE FROM sessions WHERE id = ?", sessionID)
	return err
}
