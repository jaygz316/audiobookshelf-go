package socket

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	gosocket "github.com/zishang520/socket.io/v2/socket"

	"audiobookshelf/internal/core"
	"audiobookshelf/internal/db"
	log "audiobookshelf/internal/logger"
)

// AuthenticateSocket validates the JWT token and binds the user session to the socket.
func (sa *Authority) AuthenticateSocket(client *gosocket.Socket, token string) {
	userSession, err := sa.resolveUserSession(token)
	if err != nil {
		log.Printf("[Socket] Auth failed: %v", err)
		client.Emit("auth_failed", map[string]string{"message": "Invalid token"})
		return
	}

	sa.mu.Lock()
	socketClient, exists := sa.clients[string(client.Id())]
	if exists {
		socketClient.User = userSession
	}
	sa.mu.Unlock()

	if !exists {
		log.Printf("[Socket] No client found for socket ID %s", client.Id())
		return
	}

	createdAtMs, lastSeenMs, err := sa.updateUserActivity(userSession.ID)
	if err != nil {
		log.Printf("[Socket] Failed to update user activity: %v", err)
	}

	sa.broadcastUserOnline(client, userSession, createdAtMs, lastSeenMs)
}

// resolveUserSession decodes the token and retrieves the corresponding session.
func (sa *Authority) resolveUserSession(token string) (*core.UserSession, error) {
	tokenSecret := db.GetTokenSecret(sa.database)
	userID, err := validateToken(token, tokenSecret)
	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	userSession, err := db.GetUserByIDOrOldID(sa.database, userID)
	if err != nil {
		return nil, fmt.Errorf("user lookup failed: %w", err)
	}

	if !userSession.IsActive {
		return nil, fmt.Errorf("user %s is inactive", userSession.Username)
	}

	return userSession, nil
}

// updateUserActivity updates user activity timestamps in the database.
func (sa *Authority) updateUserActivity(userID string) (createdAtMs int64, lastSeenMs int64, err error) {
	var createdAtStr, lastSeenStr sql.NullString
	err = sa.database.QueryRow("SELECT createdAt, lastSeen FROM users WHERE id = ?", userID).Scan(&createdAtStr, &lastSeenStr)
	if err != nil {
		return 0, 0, err
	}

	if createdAtStr.Valid && createdAtStr.String != "" {
		if t, err := db.ParseSQLiteTime(createdAtStr.String); err == nil {
			createdAtMs = t.UnixMilli()
		}
	}
	if lastSeenStr.Valid && lastSeenStr.String != "" {
		if t, err := db.ParseSQLiteTime(lastSeenStr.String); err == nil {
			lastSeenMs = t.UnixMilli()
		}
	} else {
		lastSeenMs = time.Now().UnixMilli()
	}

	nowStr := time.Now().UTC().Format("2006-01-02 15:04:05.000 +00:00")
	_, err = sa.database.Exec("UPDATE users SET lastSeen = ?, updatedAt = ? WHERE id = ?", nowStr, nowStr, userID)
	return createdAtMs, lastSeenMs, err
}

// broadcastUserOnline handles post-login user broadcasting and client initialization.
func (sa *Authority) broadcastUserOnline(client *gosocket.Socket, userSession *core.UserSession, createdAtMs, lastSeenMs int64) {
	log.Printf("[SocketAuthority] User Online: %s", userSession.Username)

	// Broadcast user_online to admins
	onlineUser := PublicUser{
		ID:        userSession.ID,
		Username:  userSession.Username,
		Type:      userSession.Type,
		Session:   nil,
		LastSeen:  lastSeenMs,
		CreatedAt: createdAtMs,
	}
	sa.AdminEmitter("user_online", onlineUser)

	// Emit init event
	initialPayload := map[string]interface{}{
		"userId":           userSession.ID,
		"username":         userSession.Username,
		"playbackSessions": sa.getPlaybackSessionsForUser(userSession.ID),
	}
	if userSession.Type == "root" || userSession.Type == "admin" {
		initialPayload["usersOnline"] = sa.GetUsersOnline()
	}
	client.Emit("init", initialPayload)
}

// validateToken parses the JWT token using standard auth claims.
func validateToken(tokenStr string, tokenSecret string) (string, error) {
	claims := &core.AuthClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(tokenSecret), nil
	})
	if err != nil || !token.Valid {
		return "", fmt.Errorf("invalid token: %v", err)
	}
	if claims.Type == "refresh" {
		return "", fmt.Errorf("invalid token type: refresh token not allowed")
	}
	return claims.UserID, nil
}

// ValidateToken is an exported wrapper for JWT token validation.
func ValidateToken(tokenStr string, tokenSecret string) (string, error) {
	return validateToken(tokenStr, tokenSecret)
}
