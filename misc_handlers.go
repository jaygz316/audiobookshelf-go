package main

import (
	"database/sql"
	"net/http"
)

// handleGetApiKeys returns a mock list of API keys.
func handleGetApiKeys(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiKeys":[]}`))
	}
}

// handleGetNotifications returns mock notification settings.
func handleGetNotifications(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":null,"settings":{"appriseApiUrl":null,"maxNotificationQueue":25,"maxFailedAttempts":5,"notifications":[]}}`))
	}
}

// handleGetEmailsSettings returns mock email settings.
func handleGetEmailsSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"settings":{"host":"","port":465,"secure":true,"rejectUnauthorized":true,"user":"","pass":"","testAddress":"","fromAddress":"","ereaderDevices":[]}}`))
	}
}

// handleGetFeeds returns a mock list of feeds.
func handleGetFeeds(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"feeds":[]}`))
	}
}
