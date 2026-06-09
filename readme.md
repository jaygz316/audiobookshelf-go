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

The project has been split into a high-performance Go backend and a Nuxt.js/Vue.js frontend SPA.

### 1. Go Backend (Root Directory & `internal/`)
- [main.go](file:///home/jay/projects/audiobookshelf-go/main.go): Server entry point. Sets up the configuration, connects to the SQLite database, establishes the HTTP routing mux, and orchestrates the HLS stream manager, socket.io handlers, and file watch routines.
- [db.go](file:///home/jay/projects/audiobookshelf-go/db.go): SQLite connection setup and database migration scripts.
- [auth.go](file:///home/jay/projects/audiobookshelf-go/auth.go) & [users.go](file:///home/jay/projects/audiobookshelf-go/users.go): Handles user credentials, password encryption (`bcrypt`), user access levels, sessions, and OpenID Connect (OIDC).
- [hls.go](file:///home/jay/projects/audiobookshelf-go/hls.go): Handlers for on-the-fly audio transcoding and HLS streaming using `ffmpeg` and `ffprobe`.
- [socket.go](file:///home/jay/projects/audiobookshelf-go/socket.go): Real-time communication namespace utilizing `socket.io` for status syncs, user presence, and library events.
- [scanner.go](file:///home/jay/projects/audiobookshelf-go/scanner.go): Multi-threaded library scanner that traverses audio files, parses metadata tags, and indexes authors, narrators, and series.
- [watcher.go](file:///home/jay/projects/audiobookshelf-go/watcher.go): Filesystem monitor utilizing `fsnotify` to track changes in media directories and automatically update the database.
- [internal/](file:///home/jay/projects/audiobookshelf-go/internal/): Core subpackages handling domains like RSS feeds, playlists, search providers, share links, and push notifications.

### 2. Frontend Client (`client/`)
- A single-page application built on Vue 2 / Nuxt.js, located in the [client/](file:///home/jay/projects/audiobookshelf-go/client/) folder.
- Communicates with the Go backend via a JSON REST API and Socket.io connections.
- Compiles down to static assets inside `client/dist/`, which are then served directly by the Go backend server using native Go SPA fallback routing.

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
- Go 1.22+ installed
- Node.js (version 20+) & npm
- `ffmpeg` and `ffprobe` installed and added to your system's PATH

#### Step 1: Build the Frontend Assets
Go to the `client` directory, install packages, and compile the static build:
```sh
cd client
npm ci
npm run generate
cd ..
```
This will compile the frontend and place the static assets in `client/dist/`.

#### Step 2: Build the Go Server
Run Go compiler to build the single binary:
```sh
go build -o abs-gateway .
```

#### Step 3: Run the Application
Start the Go gateway:
```sh
./abs-gateway --port=13378 --config="./config" --metadata="./metadata"
```
Or you can use environment variables:
```sh
PORT=13378 CONFIG_PATH="./config" METADATA_PATH="./metadata" ./abs-gateway
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
