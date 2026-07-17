package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (h *OIDCHandler) isValidRedirectURI(uri string) bool {
	if uri == "" {
		return false
	}
	settings := h.getSettings()
	for _, allowed := range settings.MobileRedirectURIs {
		if allowed == "*" || allowed == uri {
			return true
		}
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if host == "localhost" || host == "127.0.0.1" {
		return true
	}
	return false
}

func (h *OIDCHandler) getScopes() []string {
	settings := h.getSettings()
	scopes := []string{"openid", "profile", "email"}
	if settings.GroupClaim != "" {
		scopes = append(scopes, settings.GroupClaim)
	}
	if settings.AdvancedPermsClaim != "" {
		scopes = append(scopes, settings.AdvancedPermsClaim)
	}
	return scopes
}

func (h *OIDCHandler) getRedirectURL(r *http.Request, isMobile bool) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}

	host := r.Host
	if xfh := r.Header.Get("X-Forwarded-Host"); xfh != "" {
		host = xfh
	}

	var subfolder string
	settings := h.getSettings()
	if settings.SubfolderForRedirectURLs {
		path := r.URL.Path
		if idx := strings.Index(path, "/auth/openid"); idx != -1 {
			subfolder = path[:idx]
		}
	}

	suffix := "/auth/openid/callback"
	if isMobile {
		suffix = "/auth/openid/mobile-redirect"
	}

	return fmt.Sprintf("%s://%s%s%s", scheme, host, subfolder, suffix)
}

func generateRandomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func generateVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func s256Challenge(verifier string) string {
	sha := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sha[:])
}

func mapGroupClaims(gVal interface{}) (string, error) {
	var rawGroups []string
	switch val := gVal.(type) {
	case []interface{}:
		for _, item := range val {
			if str, ok := item.(string); ok {
				rawGroups = append(rawGroups, str)
			}
		}
	case []string:
		rawGroups = val
	case string:
		if strings.Contains(val, ",") {
			for _, part := range strings.Split(val, ",") {
				rawGroups = append(rawGroups, strings.TrimSpace(part))
			}
		} else {
			rawGroups = append(rawGroups, val)
		}
	}

	highestRole := ""
	for _, g := range rawGroups {
		gLower := strings.ToLower(strings.TrimSpace(g))
		if gLower == "admin" {
			if highestRole != "admin" {
				highestRole = "admin"
			}
		} else if gLower == "user" {
			if highestRole != "admin" && highestRole != "user" {
				highestRole = "user"
			}
		} else if gLower == "guest" {
			if highestRole == "" {
				highestRole = "guest"
			}
		}
	}

	if highestRole == "" {
		return "", fmt.Errorf("user does not belong to any matching group ('admin', 'user', or 'guest')")
	}

	return highestRole, nil
}
