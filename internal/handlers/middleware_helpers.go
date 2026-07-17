package handlers

import (
	"database/sql"
	"net/http"
	"os"
	"regexp"
	"sync"

	idb "audiobookshelf/internal/db"
)

var (
	coverRegex = regexp.MustCompile(`(?i)/api/items/[^/]+/cover/?$`)
)

// authNotNeeded checks if a request does not require authentication
// Keep it in auth middleware file where it is used.
func authNotNeeded(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	return coverRegex.MatchString(r.URL.Path)
}

var tokenSecretCache string
var tokenSecretCacheMu sync.RWMutex

func getTokenSecret(db *sql.DB) string {
	if envSecret := os.Getenv("JWT_SECRET_KEY"); envSecret != "" {
		return envSecret
	}
	tokenSecretCacheMu.RLock()
	cached := tokenSecretCache
	tokenSecretCacheMu.RUnlock()
	if cached != "" {
		return cached
	}
	if db == nil {
		return ""
	}
	secret := idb.GetTokenSecret(db)
	if secret != "" {
		tokenSecretCacheMu.Lock()
		tokenSecretCache = secret
		tokenSecretCacheMu.Unlock()
	}
	return secret
}
