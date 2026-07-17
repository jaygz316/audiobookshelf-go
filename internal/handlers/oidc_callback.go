package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/utils"
)

// handleOIDCCallback handles the incoming OIDC redirection callback flow, matching claims
// to local users, generating JWT sessions, and returning the user login payload.
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

		// Generate access token (expiring)
		secret := getTokenSecret(db)
		accessClaims := &core.AuthClaims{
			UserID:   u.ID,
			Username: u.Username,
			Type:     "access",
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        uuid.New().String(),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(2 * time.Hour)),
				Issuer:    "audiobookshelf",
			},
		}
		accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString([]byte(secret))
		if err != nil {
			log.Errorf("[OIDC Callback] Failed to sign access token: %v", err)
			http.Error(w, "Failed to login", http.StatusInternalServerError)
			return
		}

		// Generate refresh token
		refreshClaims := &core.AuthClaims{
			UserID:   u.ID,
			Username: u.Username,
			Type:     "refresh",
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        uuid.New().String(),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
				Issuer:    "audiobookshelf",
			},
		}
		refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte(secret))
		if err != nil {
			log.Errorf("[OIDC Callback] Failed to sign refresh token: %v", err)
			http.Error(w, "Failed to login", http.StatusInternalServerError)
			return
		}

		// Save session
		ipAddress := utils.GetClientIP(r)
		userAgent := r.Header.Get("User-Agent")
		expiresAt := time.Now().Add(30 * 24 * time.Hour)

		if err := idb.CreateSession(r.Context(), db, u.ID, ipAddress, userAgent, refreshToken, expiresAt); err != nil {
			log.Errorf("[OIDC Callback] Failed to create session: %v", err)
			http.Error(w, "Failed to login", http.StatusInternalServerError)
			return
		}

		// Set Cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "refresh_token",
			Value:    refreshToken,
			Path:     "/",
			MaxAge:   30 * 24 * 60 * 60,
			HttpOnly: true,
		})

		if cbURL != "" {
			redirectURL := cbURL + "?setToken=" + url.QueryEscape(accessToken) + "&accessToken=" + url.QueryEscape(accessToken)
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
				"token":       u.Token,
				"accessToken": accessToken,
				"isActive":    u.IsActive,
				"isLocked":    u.IsLocked,
				"permissions": json.RawMessage(u.Permissions),
			},
		})
	}
}
