package main

import (
	"database/sql"
	"net/http"

	isocket "audiobookshelf/internal/socket"
)

// PublicUser is an alias for the internal type.
type PublicUser = isocket.PublicUser

// OnlineUser is an alias for the internal type.
type OnlineUser = isocket.OnlineUser

// SocketClient is an alias for the internal type.
type SocketClient = isocket.SocketClient

// SocketAuthority is an alias for the internal socket Authority.
type SocketAuthority = isocket.Authority

// SocketAuth is the global reference to the socket authority.
var SocketAuth *SocketAuthority

// NewSocketAuthority creates a new instance of SocketAuthority.
func NewSocketAuthority(db *sql.DB) *SocketAuthority {
	return isocket.NewAuthority(db)
}

// InitSocketAuthority initializes the global Socket.IO server and handlers.
func InitSocketAuthority(db *sql.DB) http.Handler {
	sa := isocket.NewAuthority(db)
	SocketAuth = sa
	isocket.GlobalAuth = sa
	return isocket.InitSocketAuthority(sa)
}

// validateToken delegates to the internal socket package.
func validateToken(tokenStr string, tokenSecret string) (string, error) {
	return isocket.ValidateToken(tokenStr, tokenSecret)
}
