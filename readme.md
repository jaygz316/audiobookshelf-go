<br />
<div align="center">
   <img alt="Audiobookshelf Banner" src="https://github.com/advplyr/audiobookshelf/raw/master/images/banner.svg" width="600">

  <p align="center">
    <br />
    <strong>A heavily AI refactored version of Audiobookshelf written in Go for fun.</strong>
  </p>
</div>

# About

This project is a heavily AI refactored version of **[Audiobookshelf](https://github.com/advplyr/audiobookshelf)** written in **Go** for fun and performance. It replicates the core server features of Audiobookshelf (metadata fetching, HLS streaming, user progress synchronization, HLS transcoding via FFmpeg, and playlists) in a lightweight, native Go application.

### Features

- Fully **open-source**, compatible with the [Android & iOS app](https://github.com/advplyr/audiobookshelf-app)
- Stream all audio formats on the fly with custom HLS transcoding using `ffmpeg`
- Multi-user support with custom permissions
- Keeps progress per user and syncs across devices in real-time using Socket.io
- Auto-detects library updates using a fast Go filesystem watcher (`fsnotify`)
- Backup your metadata + automated daily backups
- Progressive Web App (PWA) client
- Built-in chapter editor and metadata editor
- Pure Go SQLite database access using `modernc.org/sqlite` (no CGO required!)

---

# Codebase Explanation

The project is structured as a high-performance Go backend that embeds a pre-built Nuxt/Vue frontend single-page application (SPA).

### 1. Go Backend (Root Directory & `internal/`)
- [main.go](file:///home/jay/projects/audiobookshelf-go/main.go): Server entry point. Sets up the configuration, connects to the SQLite database, establishes the HTTP routing mux, and embeds the frontend and documentation.
- [internal/handlers/](file:///home/jay/projects/audiobookshelf-go/internal/handlers/): Contains HTTP route handlers (defined in `routes.go`), authentication/authorization middleware, library operations, settings, and other API endpoints.
- [internal/db/](file:///home/jay/projects/audiobookshelf-go/internal/db/): SQLite connection setup, migrations, database schemas, and data queries.
- [internal/auth/](file:///home/jay/projects/audiobookshelf-go/internal/auth/): Handles password hashing (`bcrypt`), user credentials, session verification, and OIDC support.
- [internal/hls/](file:///home/jay/projects/audiobookshelf-go/internal/hls/): Audio transcoding and HLS stream segmenting utilizing `ffmpeg` and `ffprobe`.
- [internal/socket/](file:///home/jay/projects/audiobookshelf-go/internal/socket/): Real-time communication via Socket.io for device progress syncing, library scans, and active user/session tracking.
- [internal/scanner/](file:///home/jay/projects/audiobookshelf-go/internal/scanner/): Multi-threaded library scanner that traverses audio directories, parses file metadata tags (ID3, Vorbis, etc.), and indexes items.
- [internal/watcher/](file:///home/jay/projects/audiobookshelf-go/internal/watcher/): Filesystem monitor utilizing `fsnotify` to track changes in media folders and automatically queue library rescans.
- [internal/](file:///home/jay/projects/audiobookshelf-go/internal/): Core subpackages handling specialized modules like backups (`internal/backup/`), RSS feeds (`internal/feed/`), share links (`internal/share/`), push notifications (`internal/notification/`), and search providers (`internal/providers/`).

### 2. Frontend Client (`frontend/`)
- A single-page application built with Nuxt.js/Vue.js. The static assets are pre-built and stored in the [frontend/](file:///home/jay/projects/audiobookshelf-go/frontend/) folder.
- The assets are embedded directly into the Go binary at compile-time using Go's `//go:embed` directive.
- The Go server hosts these files directly using SPA fallback routing, and the client communicates with the backend via a JSON REST API and real-time Socket.io connections.

---

# How to Deploy

### 1. Deploying via Docker (Recommended)
This codebase includes a multi-stage `Dockerfile` that builds the Nuxt client, compiles the Go backend binary as a static target, and produces a minimal runtime image.

#### Prerequisites
- Docker and Docker Compose installed.
- Ensure `ffmpeg` and `ffprobe` are available (automatically included in the Docker image).

#### Start with Docker Compose
We provide a [docker-compose.yml](file:///home/jay/projects/audiobookshelf-go/docker-compose.yml) in the root of the project:
```yaml
services:
  audiobookshelf:
    build:
      context: .
      dockerfile: Dockerfile
    ports:
      - 13378:80
    volumes:
      - ./audiobooks:/audiobooks
      - ./podcasts:/podcasts
      - ./metadata:/metadata
      - ./config:/config
    restart: unless-stopped
```

To build and start the container, run:
```sh
docker compose up -d --build
```
The application will be accessible at `http://localhost:13378`.

---

### 2. Running & Building from Source (Bare Metal)

#### Prerequisites
- Go 1.26+ installed (configured in [go.mod](file:///home/jay/projects/audiobookshelf-go/go.mod))
- `ffmpeg` and `ffprobe` installed and added to your system's PATH

#### Step 1: Frontend Assets (Pre-built)
The frontend is already pre-compiled inside the [frontend/](file:///home/jay/projects/audiobookshelf-go/frontend/) directory and will be embedded during the Go build step. No Node.js build steps are required.

#### Step 2: Build the Go Server
Run Go compiler to build the server binary (either named `audiobookshelf` or `abs-gateway`):
```sh
go build -o audiobookshelf .
```

#### Step 3: Run the Application
Start the Go gateway:
```sh
./audiobookshelf --port=13378 --config="./config" --metadata="./metadata"
```
Or you can use environment variables:
```sh
PORT=13378 CONFIG_PATH="./config" METADATA_PATH="./metadata" ./audiobookshelf
```

---

# Reverse Proxy Setup

#### Important! Audiobookshelf requires a WebSocket connection.

### NGINX Proxy Manager
Toggle Websockets support.

### NGINX Reverse Proxy
Add this to the site config file on your nginx server after you have changed the relevant parts in the `<>` brackets, and inserted your certificate paths:

```nginx
server {
   listen 443 ssl;
   server_name <sub>.<domain>.<tld>;

   access_log /var/log/nginx/audiobookshelf.access.log;
   error_log /var/log/nginx/audiobookshelf.error.log;

   ssl_certificate      /path/to/certificate;
   ssl_certificate_key  /path/to/key;

   location / {
      proxy_set_header X-Forwarded-For    $proxy_add_x_forwarded_for;
      proxy_set_header X-Forwarded-Proto  $scheme;
      proxy_set_header Host               $http_host;
      proxy_set_header Upgrade            $http_upgrade;
      proxy_set_header Connection         "upgrade";

      proxy_http_version                  1.1;

      proxy_pass                          http://localhost:13378;
      proxy_redirect                      http:// https://;

      # Prevent 413 Request Entity Too Large error
      client_max_body_size                10240M;
   }
}
```

### Apache Reverse Proxy
Add this to the site config file on your Apache server. Make sure you enable the following modules: `ssl`, `proxy`, `proxy_http`, `proxy_wstunnel`, and `rewrite`.

```apache
<IfModule mod_ssl.c>
<VirtualHost *:443>
    ServerName <sub>.<domain>.<tld>

    ErrorLog ${APACHE_LOG_DIR}/error.log
    CustomLog ${APACHE_LOG_DIR}/access.log combined

    ProxyPreserveHost On
    ProxyPass / http://localhost:13378/
    RewriteEngine on
    RewriteCond %{HTTP:Upgrade} websocket [NC]
    RewriteCond %{HTTP:Connection} upgrade [NC]
    RewriteRule ^/?(.*) "ws://localhost:13378/$1" [P,L]

    SSLCertificateFile /path/to/cert/file
    SSLCertificateKeyFile /path/to/key/file
</VirtualHost>
</IfModule>
```

### Caddy
```caddy
subdomain.domain.com {
    encode gzip zstd
    reverse_proxy localhost:13378
}
```

---

# Organizing your audiobooks

Directory structure and folder names are important to Audiobookshelf. For details on supported directory layouts, folder naming conventions, and audio file metadata usage, please refer to the [official Audiobookshelf directory structure guide](https://audiobookshelf.org/docs#book-directory-structure).
