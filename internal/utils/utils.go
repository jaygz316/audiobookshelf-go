package utils

import (
	"github.com/google/uuid"
)

// UUIDStr returns a new UUID string.
func UUIDStr() string {
	return uuid.New().String()
}
