package handlers

import (
	"database/sql"
	"net/http"

	"audiobookshelf/internal/core"
)

func registerAuthAndUserRoutes(mux *http.ServeMux, cfg *core.Config, db *sql.DB, appRoot string) {
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/login"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			RateLimitMiddleware(LoginRateLimiter)(http.HandlerFunc(handleLogin(db))).ServeHTTP(w, r)
		} else if r.Method == http.MethodGet {
			r.URL.Path = joinPath(cfg.RouterBasePath, "/index.html")
			serveStaticOrSPA(subFS, cfg.RouterBasePath)(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/logout"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleLogout(db)(w, r)
		} else if r.Method == http.MethodGet {
			r.URL.Path = joinPath(cfg.RouterBasePath, "/index.html")
			serveStaticOrSPA(subFS, cfg.RouterBasePath)(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/init"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			RateLimitMiddleware(LoginRateLimiter)(http.HandlerFunc(handleInit(db))).ServeHTTP(w, r)
		} else if r.Method == http.MethodGet {
			r.URL.Path = joinPath(cfg.RouterBasePath, "/index.html")
			serveStaticOrSPA(subFS, cfg.RouterBasePath)(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/auth/logout"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleLogout(db)(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/auth/refresh"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleRefresh(db)(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/authorize"), func(w http.ResponseWriter, r *http.Request) {
		AuthMiddlewareWrapper(db, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost || r.Method == http.MethodGet {
				handleAuthorize(db)(w, r)
			} else {
				http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			}
		})).ServeHTTP(w, r)
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/v1/session"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetSession(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/session"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetSession(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/users/online"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetOnlineUsers(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	handleUsersDispatch := func(db *sql.DB) http.HandlerFunc {
		usersHandler := handleGetUsers(db)
		crudHandler := handleUserCRUD(db)
		return func(w http.ResponseWriter, r *http.Request) {
			pathWithoutPrefix := trimBasePath(r.URL.Path, cfg.RouterBasePath)
			if pathWithoutPrefix == "/api/users" || pathWithoutPrefix == "/api/users/" {
				if r.Method == http.MethodGet {
					usersHandler(w, r)
					return
				}
			}
			crudHandler(w, r)
		}
	}

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/users"), func(w http.ResponseWriter, r *http.Request) {
		AuthMiddlewareWrapper(db, handleUsersDispatch(db)).ServeHTTP(w, r)
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/users/"), func(w http.ResponseWriter, r *http.Request) {
		AuthMiddlewareWrapper(db, handleUsersDispatch(db)).ServeHTTP(w, r)
	})
}
