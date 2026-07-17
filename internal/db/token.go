package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"time"

	log "audiobookshelf/internal/logger"
)

// GetTokenSecret retrieves the JWT token secret from the environment or database settings.
// If no secret is configured, it automatically generates a secure 32-byte random hex secret (256-bit entropy)
// and persists it to the server settings in the database.
func GetTokenSecret(database *sql.DB) string {
	if envSecret := os.Getenv("JWT_SECRET_KEY"); envSecret != "" {
		return envSecret
	}
	if database == nil {
		return ""
	}
	settings, err := GetServerSettings(database)
	if err == nil && settings != nil && settings.TokenSecret != "" {
		return settings.TokenSecret
	}

	// Generate secure 32-byte hex secret
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Errorf("Failed to generate secure random secret: %v", err)
		return ""
	}
	secret := hex.EncodeToString(b)

	if settings == nil {
		settings = &ServerSettings{}
	}
	settings.TokenSecret = secret

	newValBytes, err := json.Marshal(settings)
	if err != nil {
		log.Errorf("Failed to marshal settings: %v", err)
		return ""
	}

	nowStr := TimeToDBStr(time.Now())
	_, err = database.Exec("INSERT INTO settings (key, value, createdAt, updatedAt) VALUES ('server-settings', ?, ?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updatedAt=excluded.updatedAt",
		string(newValBytes), nowStr, nowStr)
	if err != nil {
		log.Errorf("Failed to save secure token secret: %v", err)
		return ""
	}

	log.Info("Successfully generated and saved new secure JWT token secret to database")
	return secret
}
