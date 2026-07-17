package share

import (
	"context"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// ValidateSharePassword matches a plaintext password with the hashed credentials.
func (m *ShareManager) ValidateSharePassword(ctx context.Context, id, password string) (bool, error) {
	s, err := m.GetShare(ctx, id)
	if err != nil {
		return false, fmt.Errorf("failed to retrieve share link for validation: %w", err)
	}
	if s == nil {
		return false, nil
	}

	if s.PasswordHash == "" {
		// No password protection configured
		return true, nil
	}

	err = bcrypt.CompareHashAndPassword([]byte(s.PasswordHash), []byte(password))
	if err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return false, nil
		}
		return false, fmt.Errorf("failed to validate password: %w", err)
	}

	return true, nil
}
