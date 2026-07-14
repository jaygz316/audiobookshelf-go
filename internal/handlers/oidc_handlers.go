package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"audiobookshelf/internal/auth"
	idb "audiobookshelf/internal/db"
)

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
		IssuerURL:    getString(settingsMap["authOpenIDIssuerURL"]),
		ClientID:     getString(settingsMap["authOpenIDClientID"]),
		ClientSecret: getString(settingsMap["authOpenIDClientSecret"]),
		RedirectURL:  getString(settingsMap["authOpenIDRedirectURL"]),
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

func getString(val interface{}) string {
	if val == nil {
		return ""
	}
	if s, ok := val.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", val)
}

func handleOIDCCallback(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		globalOIDCHandlerMu.RLock()
		handler := globalOIDCHandler
		globalOIDCHandlerMu.RUnlock()

		if handler == nil {
			http.Error(w, "No active OIDC session", http.StatusBadRequest)
			return
		}

		claims, err := handler.HandleCallback(w, r)
		if err != nil {
			log.Errorf("[OIDC Callback] Error: %v", err)
			http.Error(w, fmt.Sprintf("Authentication failed: %v", err), http.StatusUnauthorized)
			return
		}

		if claims == nil {
			return
		}

		s, err := getOIDCSettings(db)
		if err != nil {
			http.Error(w, "Failed to load OIDC settings", http.StatusInternalServerError)
			return
		}

		u, err := idb.FindUserFromOpenIdUserInfo(r.Context(), db, claims, s.MatchExistingBy)
		if err != nil {
			log.Errorf("[OIDC Callback] idb.User match error: %v", err)
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		if mapped, ok := claims["mapped_role"].(string); ok && mapped != "" {
			if u == nil {
				if !s.AutoRegister {
					http.Error(w, "Auto-registration is disabled and no matching user was found", http.StatusUnauthorized)
					return
				}
				u, err = idb.CreateUserFromOpenIdUserInfo(r.Context(), db, claims, getTokenSecret(db), mapped)
				if err != nil {
					log.Errorf("[OIDC Callback] idb.User registration failed: %v", err)
					http.Error(w, "Failed to register user", http.StatusInternalServerError)
					return
				}
			} else {
				if u.Type != mapped {
					err = idb.UpdateUserTypeAndToken(r.Context(), db, u, mapped, getTokenSecret(db))
					if err != nil {
						log.Errorf("[OIDC Callback] Failed to update user type and token: %v", err)
						http.Error(w, "Failed to update user type", http.StatusInternalServerError)
						return
					}
				}
			}
		} else {
			if u == nil {
				if !s.AutoRegister {
					http.Error(w, "Auto-registration is disabled and no matching user was found", http.StatusUnauthorized)
					return
				}
				u, err = idb.CreateUserFromOpenIdUserInfo(r.Context(), db, claims, getTokenSecret(db), "user")
				if err != nil {
					log.Errorf("[OIDC Callback] idb.User registration failed: %v", err)
					http.Error(w, "Failed to register user", http.StatusInternalServerError)
					return
				}
			}
		}

		var cbURL string
		if cookie, err := r.Cookie("auth_cb"); err == nil {
			cbURL = cookie.Value
		}

		tokenString := u.Token

		if cbURL != "" {
			redirectURL := cbURL + "?setToken=" + url.QueryEscape(tokenString) + "&accessToken=" + url.QueryEscape(tokenString)
			if stateCookie, err := r.Cookie("auth_state"); err == nil {
				redirectURL += "&state=" + url.QueryEscape(stateCookie.Value)
			}
			http.Redirect(w, r, redirectURL, http.StatusFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"user": map[string]interface{}{
				"id":          u.ID,
				"username":    u.Username,
				"email":       u.Email,
				"type":        u.Type,
				"token":       tokenString,
				"isActive":    u.IsActive,
				"isLocked":    u.IsLocked,
				"permissions": json.RawMessage(u.Permissions),
			},
		})
	}
}
