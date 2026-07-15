package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"

	idb "audiobookshelf/internal/db"
)

var (
	allowIframeCache   bool
	allowIframeCached  bool
	allowIframeCacheMu sync.RWMutex
)

func getAllowIframeSetting(db *sql.DB) bool {
	allowIframeCacheMu.RLock()
	if allowIframeCached {
		val := allowIframeCache
		allowIframeCacheMu.RUnlock()
		return val
	}
	allowIframeCacheMu.RUnlock()

	allowIframeCacheMu.Lock()
	defer allowIframeCacheMu.Unlock()
	if allowIframeCached {
		return allowIframeCache
	}

	allowIframe := false
	if db != nil {
		if settings, err := idb.GetServerSettings(db); err == nil && settings != nil {
			allowIframe = settings.AllowIframe
		}
	}
	allowIframeCache = allowIframe
	allowIframeCached = true
	return allowIframeCache
}

func InvalidateAllowIframeCache() {
	allowIframeCacheMu.Lock()
	allowIframeCached = false
	allowIframeCacheMu.Unlock()
}

func serveStaticOrSPA(fSys fs.FS, routerBasePath string) http.HandlerFunc {
	if fSys == nil {
		// Fallback to frontend directory FS if subFS is nil
		fSys = os.DirFS("frontend")
	}

	fileServer := http.FileServer(http.FS(fSys))
	return func(w http.ResponseWriter, r *http.Request) {
		allowIframe := getAllowIframeSetting(globalDB)
		if !allowIframe {
			w.Header().Set("X-Frame-Options", "SAMEORIGIN")
			w.Header().Set("Content-Security-Policy", "frame-ancestors 'self'")
		}

		reqPath := r.URL.Path
		if strings.HasPrefix(reqPath, routerBasePath) {
			reqPath = strings.TrimPrefix(reqPath, routerBasePath)
		}
		if reqPath == "" {
			reqPath = "/"
		}

		cleanedPath := path.Clean("/" + reqPath)
		if cleanedPath == "/" {
			cleanedPath = "."
		} else {
			cleanedPath = cleanedPath[1:]
		}

		if cleanedPath == "index.html" {
			data, err := fs.ReadFile(fSys, "index.html")
			if err == nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(data)
				return
			}
		}

		file, err := fSys.Open(cleanedPath)
		var isDir bool
		if err == nil {
			stat, statErr := file.Stat()
			if statErr == nil && stat.IsDir() {
				isDir = true
			}
			file.Close()
		}

		if err == nil && !isDir {
			http.StripPrefix(routerBasePath, fileServer).ServeHTTP(w, r)
			return
		}

		// Hardening: Fallback to index.html only for GET/HEAD requests.
		// For any other method, return 404.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error": "API route not found"}`))
			return
		}

		// Hardening: If the file was not found and has a typical static asset file extension,
		// return a 404 immediately instead of serving index.html.
		ext := strings.ToLower(path.Ext(cleanedPath))
		if ext != "" {
			assetExtensions := map[string]bool{
				".js":          true,
				".css":         true,
				".png":         true,
				".jpg":         true,
				".jpeg":        true,
				".gif":         true,
				".ico":         true,
				".svg":         true,
				".json":        true,
				".woff":        true,
				".woff2":       true,
				".ttf":         true,
				".map":         true,
				".webmanifest": true,
				".mp3":         true,
				".m4b":         true,
				".m4a":         true,
				".epub":        true,
				".pdf":         true,
			}
			if assetExtensions[ext] {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error": "Static asset not found"}`))
				return
			}
		}

		// Serve index.html as fallback for Client-side SPA routing
		log.Infof("[SPA] Fallback for GET %s -> index.html", r.URL.Path)
		data, err := fs.ReadFile(fSys, "index.html")
		if err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>Audiobookshelf Go Gateway</body></html>"))
	}
}
