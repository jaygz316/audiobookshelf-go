package handlers

import (
	"database/sql"
	"encoding/json"
	"time"

	idb "audiobookshelf/internal/db"
)

type EreaderDevice struct {
	Name               string   `json:"name"`
	Email              string   `json:"email"`
	AvailabilityOption string   `json:"availabilityOption"` // "adminOrUp", "specificUsers", "allUsers"
	Users              []string `json:"users"`              // User IDs
}

type EmailSettings struct {
	ID                 string          `json:"id"` // "email-settings"
	Host               string          `json:"host"`
	Port               int             `json:"port"`
	Secure             bool            `json:"secure"`
	RejectUnauthorized bool            `json:"rejectUnauthorized"`
	User               string          `json:"user"`
	Pass               string          `json:"pass"`
	TestAddress        string          `json:"testAddress"`
	FromAddress        string          `json:"fromAddress"`
	EreaderDevices     []EreaderDevice `json:"ereaderDevices"`
}

type EmailTestRequest struct {
	Host               string `json:"host"`
	Port               int    `json:"port"`
	Secure             bool   `json:"secure"`
	RejectUnauthorized bool   `json:"rejectUnauthorized"`
	User               string `json:"user"`
	Pass               string `json:"pass"`
	TestAddress        string `json:"testAddress"`
	FromAddress        string `json:"fromAddress"`
}

func defaultEmailSettings() *EmailSettings {
	return &EmailSettings{
		ID:                 "email-settings",
		Host:               "",
		Port:               587,
		Secure:             false,
		RejectUnauthorized: true,
		User:               "",
		Pass:               "",
		TestAddress:        "",
		FromAddress:        "",
		EreaderDevices:     []EreaderDevice{},
	}
}

func loadEmailSettings(db *sql.DB) (*EmailSettings, error) {
	var valStr string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'email-settings'").Scan(&valStr)
	if err != nil {
		return nil, err
	}
	var settings EmailSettings
	if err := json.Unmarshal([]byte(valStr), &settings); err != nil {
		return nil, err
	}
	return &settings, nil
}

func saveEmailSettings(db *sql.DB, settings *EmailSettings) error {
	newValBytes, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	nowStr := idb.TimeToDBStr(time.Now())
	_, err = db.Exec("INSERT INTO settings (key, value, createdAt, updatedAt) VALUES ('email-settings', ?, ?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updatedAt=excluded.updatedAt",
		string(newValBytes), nowStr, nowStr)
	return err
}

func sanitizePassword(pass string) string {
	if pass != "" {
		return "********"
	}
	return ""
}
