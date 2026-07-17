package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"audiobookshelf/internal/auth"
)

// getOIDCSettings retrieves OIDC configuration options from the database settings.
func getOIDCSettings(db *sql.DB) (auth.OIDCSettings, error) {
	var valStr string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
	if err != nil {
		return auth.OIDCSettings{}, err
	}

	var settingsMap map[string]interface{}
	if err := json.Unmarshal([]byte(valStr), &settingsMap); err != nil {
		return auth.OIDCSettings{}, err
	}

	s := auth.OIDCSettings{
		IssuerURL:             getString(settingsMap["authOpenIDIssuerURL"]),
		ClientID:              getString(settingsMap["authOpenIDClientID"]),
		ClientSecret:          getString(settingsMap["authOpenIDClientSecret"]),
		RedirectURL:           getString(settingsMap["authOpenIDRedirectURL"]),
		AuthorizationURL:      getString(settingsMap["authOpenIDAuthorizationURL"]),
		TokenURL:              getString(settingsMap["authOpenIDTokenURL"]),
		UserInfoURL:           getString(settingsMap["authOpenIDUserInfoURL"]),
		JwksURL:               getString(settingsMap["authOpenIDJwksURL"]),
		LogoutURL:             getString(settingsMap["authOpenIDLogoutURL"]),
		TokenSigningAlgorithm: getString(settingsMap["authOpenIDTokenSigningAlgorithm"]),
	}

	if settingsMap["authOpenIDAutoRegister"] != nil {
		if val, ok := settingsMap["authOpenIDAutoRegister"].(bool); ok {
			s.AutoRegister = val
		}
	}
	if settingsMap["authOpenIDMatchExistingBy"] != nil {
		s.MatchExistingBy = getString(settingsMap["authOpenIDMatchExistingBy"])
	}
	if settingsMap["authOpenIDMobileRedirectURIs"] != nil {
		if list, ok := settingsMap["authOpenIDMobileRedirectURIs"].([]interface{}); ok {
			for _, item := range list {
				s.MobileRedirectURIs = append(s.MobileRedirectURIs, getString(item))
			}
		}
	}
	if settingsMap["authOpenIDGroupClaim"] != nil {
		s.GroupClaim = getString(settingsMap["authOpenIDGroupClaim"])
	}
	if settingsMap["authOpenIDAdvancedPermsClaim"] != nil {
		s.AdvancedPermsClaim = getString(settingsMap["authOpenIDAdvancedPermsClaim"])
	}
	if settingsMap["authOpenIDSubfolderForRedirectURLs"] != nil {
		if val, ok := settingsMap["authOpenIDSubfolderForRedirectURLs"].(bool); ok {
			s.SubfolderForRedirectURLs = val
		}
	}

	return s, nil
}

// getString converts a generic interface value to a string.
func getString(val interface{}) string {
	if val == nil {
		return ""
	}
	if s, ok := val.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", val)
}
