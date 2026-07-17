package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"

	"audiobookshelf/internal/db"
)

func getTokenSecret(database *sql.DB) string {
	if envSecret := os.Getenv("JWT_SECRET_KEY"); envSecret != "" {
		return envSecret
	}
	if cachedSecret != "" {
		return cachedSecret
	}
	if database == nil {
		return ""
	}
	secret := db.GetTokenSecret(database)
	if secret != "" {
		cachedSecret = secret
	}
	return secret
}

func getVersion(appRoot string) string {
	pkgPath := filepath.Join(appRoot, "package.json")
	file, err := os.Open(pkgPath)
	if err != nil {
		return "2.35.1" // Fallback
	}
	defer file.Close()

	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(file).Decode(&pkg); err != nil {
		return "2.35.1"
	}
	return pkg.Version
}
