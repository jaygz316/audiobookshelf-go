package e2e_tests

import (
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var (
	compiledBinPath string
	compileErr      error
	compileOnce     sync.Once
)

// getTestBinary compiles the root main package once and returns the path to the executable.
func getTestBinary() (string, error) {
	compileOnce.Do(func() {
		tempDir, err := os.MkdirTemp("", "abs-bin-")
		if err != nil {
			compileErr = err
			return
		}
		binPath := filepath.Join(tempDir, "abs-server-test")

		// Root directory is the parent of the e2e_tests folder.
		wd, err := os.Getwd()
		if err != nil {
			compileErr = err
			return
		}
		// In case we are running inside e2e_tests
		var rootDir string
		if filepath.Base(wd) == "e2e_tests" {
			rootDir = filepath.Dir(wd)
		} else {
			rootDir = wd
		}
		cmd := exec.Command("go", "build", "-o", binPath, ".")
		cmd.Dir = rootDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			compileErr = fmt.Errorf("failed to compile server binary: %v", err)
			return
		}
		compiledBinPath = binPath
	})
	return compiledBinPath, compileErr
}

// CleanupTestBinary removes the compiled test binary and its temporary directory.
func CleanupTestBinary() {
	if compiledBinPath != "" {
		_ = os.RemoveAll(filepath.Dir(compiledBinPath))
	}
}

// TestHarness manages the lifecycle of the sandboxed server subprocess.
type TestHarness struct {
	ConfigDir   string
	MetadataDir string
	Port        int
	Cmd         *exec.Cmd
	DBPath      string
	BaseURL     string
}

// NewTestHarness creates and initializes a new TestHarness.
func NewTestHarness() *TestHarness {
	return &TestHarness{}
}

// Start boots the compiled server subprocess with sandboxed directories and database.
func (h *TestHarness) Start() error {
	// 1. Get a free port dynamically
	port, err := h.getFreePort()
	if err != nil {
		return fmt.Errorf("failed to allocate free port: %v", err)
	}
	h.Port = port
	h.BaseURL = fmt.Sprintf("http://127.0.0.1:%d/audiobookshelf", port)

	// 2. Create sandbox directories
	cfgDir, err := os.MkdirTemp("", "abs-config-")
	if err != nil {
		return fmt.Errorf("failed to create config sandbox: %v", err)
	}
	h.ConfigDir = cfgDir

	metaDir, err := os.MkdirTemp("", "abs-metadata-")
	if err != nil {
		_ = os.RemoveAll(cfgDir)
		return fmt.Errorf("failed to create metadata sandbox: %v", err)
	}
	h.MetadataDir = metaDir

	// 3. Pre-initialize the database
	h.DBPath = filepath.Join(h.ConfigDir, "absdatabase.sqlite")
	if err := h.preInitDB(); err != nil {
		h.Cleanup()
		return fmt.Errorf("database pre-initialization failed: %v", err)
	}

	// 4. Retrieve compiled binary path
	binPath, err := getTestBinary()
	if err != nil {
		h.Cleanup()
		return fmt.Errorf("failed to get compiled test binary: %v", err)
	}

	// 5. Spawn subprocess
	h.Cmd = exec.Command(binPath,
		"-c", h.ConfigDir,
		"-m", h.MetadataDir,
		"-p", fmt.Sprintf("%d", h.Port),
		"-h", "127.0.0.1",
	)

	// Route environment ROUTER_BASE_PATH if necessary (though it defaults to "/audiobookshelf")
	h.Cmd.Env = append(os.Environ(), "ROUTER_BASE_PATH=/audiobookshelf")

	// Capture output to a log file in metadata dir for debugging
	logFile, err := os.Create(filepath.Join(h.MetadataDir, "server.log"))
	if err == nil {
		h.Cmd.Stdout = logFile
		h.Cmd.Stderr = logFile
	} else {
		h.Cmd.Stdout = os.Stdout
		h.Cmd.Stderr = os.Stderr
	}

	if err := h.Cmd.Start(); err != nil {
		h.Cleanup()
		return fmt.Errorf("failed to start server subprocess: %v", err)
	}

	// 6. Probe for liveness
	if err := h.waitForLiveness(); err != nil {
		h.Stop()
		return fmt.Errorf("server did not pass health check: %v", err)
	}

	return nil
}

// Stop terminates the server subprocess and cleans up sandbox directories.
func (h *TestHarness) Stop() {
	if h.Cmd != nil && h.Cmd.Process != nil {
		// Try to terminate gracefully with os.Interrupt (SIGINT)
		_ = h.Cmd.Process.Signal(os.Interrupt)

		done := make(chan error, 1)
		go func() {
			done <- h.Cmd.Wait()
		}()

		select {
		case <-done:
			// Server terminated successfully
		case <-time.After(3 * time.Second):
			// Force kill if it doesn't respond to interrupt within 3 seconds
			_ = h.Cmd.Process.Kill()
			<-done
		}
	}
	h.Cleanup()
}

// Cleanup deletes the sandbox folders.
func (h *TestHarness) Cleanup() {
	if h.ConfigDir != "" {
		_ = os.RemoveAll(h.ConfigDir)
		h.ConfigDir = ""
	}
	if h.MetadataDir != "" {
		_ = os.RemoveAll(h.MetadataDir)
		h.MetadataDir = ""
	}
}

// getFreePort listens briefly on port :0 to find a free port from the OS.
func (h *TestHarness) getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// waitForLiveness polls /healthcheck until success or timeout.
func (h *TestHarness) waitForLiveness() error {
	url := fmt.Sprintf("%s/healthcheck", h.BaseURL)
	timeout := time.After(8 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for server to respond at %s", url)
		case <-ticker.C:
			resp, err := http.Get(url)
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
	}
}

// preInitDB sets up all 20 schema tables and a default settings record.
func (h *TestHarness) preInitDB() error {
	db, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	queries := []string{
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT,
			createdAt TEXT,
			updatedAt TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT,
			email TEXT,
			pash TEXT,
			type TEXT,
			token TEXT,
			isActive INTEGER,
			isLocked INTEGER,
			lastSeen INTEGER,
			permissions TEXT,
			bookmarks TEXT,
			extraData TEXT,
			createdAt TEXT,
			updatedAt TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS apiKeys (
			id TEXT PRIMARY KEY,
			isActive INTEGER,
			expiresAt TEXT,
			userId TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS libraries (
			id TEXT PRIMARY KEY,
			name TEXT,
			displayOrder INTEGER,
			icon TEXT,
			mediaType TEXT,
			provider TEXT,
			lastScan TEXT,
			lastScanVersion TEXT,
			settings TEXT,
			createdAt TEXT,
			updatedAt TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS libraryFolders (
			id TEXT PRIMARY KEY,
			path TEXT,
			libraryId TEXT,
			createdAt TEXT,
			updatedAt TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS libraryItems (
			id TEXT PRIMARY KEY,
			ino TEXT,
			libraryId TEXT,
			path TEXT,
			relPath TEXT,
			isFile INTEGER,
			mtime TEXT,
			ctime TEXT,
			birthtime TEXT,
			createdAt TEXT,
			updatedAt TEXT,
			isMissing INTEGER,
			isInvalid INTEGER,
			mediaType TEXT,
			mediaId TEXT,
			size INTEGER,
			libraryFolderId TEXT,
			authorNamesFirstLast TEXT,
			authorNamesLastFirst TEXT,
			title TEXT,
			titleIgnorePrefix TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS books (
			id TEXT PRIMARY KEY,
			title TEXT,
			titleIgnorePrefix TEXT,
			subtitle TEXT,
			publishedYear TEXT,
			publishedDate TEXT,
			publisher TEXT,
			description TEXT,
			isbn TEXT,
			asin TEXT,
			language TEXT,
			explicit INTEGER,
			abridged INTEGER,
			coverPath TEXT,
			duration REAL,
			narrators BLOB,
			audioFiles BLOB,
			ebookFile BLOB,
			chapters BLOB,
			tags BLOB,
			genres BLOB
		)`,
		`CREATE TABLE IF NOT EXISTS podcasts (
			id TEXT PRIMARY KEY,
			title TEXT,
			titleIgnorePrefix TEXT,
			author TEXT,
			releaseDate TEXT,
			feedURL TEXT,
			imageURL TEXT,
			description TEXT,
			itunesPageURL TEXT,
			itunesId TEXT,
			itunesArtistId TEXT,
			language TEXT,
			podcastType TEXT,
			explicit INTEGER,
			autoDownloadEpisodes INTEGER,
			autoDownloadSchedule TEXT,
			lastEpisodeCheck TEXT,
			maxEpisodesToKeep INTEGER,
			maxNewEpisodesToDownload INTEGER,
			coverPath TEXT,
			tags BLOB,
			genres BLOB,
			numEpisodes INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS bookSeries (
			bookId TEXT,
			seriesId TEXT,
			sequence TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS series (
			id TEXT PRIMARY KEY,
			libraryId TEXT,
			name TEXT,
			nameIgnorePrefix TEXT,
			description TEXT,
			createdAt TEXT,
			updatedAt TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS mediaProgresses (
			id TEXT PRIMARY KEY,
			userId TEXT,
			mediaItemId TEXT,
			mediaItemType TEXT,
			duration REAL,
			currentTime REAL,
			isFinished INTEGER,
			hideFromContinueListening INTEGER,
			ebookLocation TEXT,
			ebookProgress REAL,
			finishedAt TEXT,
			extraData TEXT,
			podcastId TEXT,
			createdAt TEXT,
			updatedAt TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS playbackSessions (
			id TEXT PRIMARY KEY,
			userId TEXT,
			mediaItemId TEXT,
			mediaItemType TEXT,
			startTime REAL,
			libraryId TEXT,
			extraData TEXT,
			timeListening REAL,
			currentTime REAL,
			serverVersion TEXT,
			coverPath TEXT,
			date TEXT,
			dayOfWeek TEXT,
			createdAt TEXT,
			updatedAt TEXT,
			deviceId TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS podcastEpisodes (
			id TEXT PRIMARY KEY,
			podcastId TEXT,
			title TEXT,
			subtitle TEXT,
			description TEXT,
			pubDate TEXT,
			audioFile TEXT,
			duration REAL,
			isFinished INTEGER,
			hideFromContinueListening INTEGER,
			finishedAt TEXT,
			createdAt TEXT,
			updatedAt TEXT,
			season TEXT,
			episode TEXT,
			episodeType TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS playlists (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			createdAt TEXT,
			updatedAt TEXT,
			libraryId TEXT,
			userId TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS playlistMediaItems (
			id TEXT PRIMARY KEY,
			mediaItemId TEXT,
			mediaItemType TEXT,
			"order" INTEGER,
			createdAt TEXT,
			playlistId TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS devices (
			id TEXT PRIMARY KEY,
			deviceId TEXT,
			clientName TEXT,
			userId TEXT,
			createdAt TEXT,
			updatedAt TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS shares (
			id TEXT PRIMARY KEY,
			userId TEXT,
			itemId TEXT,
			itemType TEXT,
			slug TEXT,
			expiration TEXT,
			maxDownloads INTEGER,
			downloadsCount INTEGER,
			createdAt TEXT,
			updatedAt TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			userId TEXT,
			ipAddress TEXT,
			userAgent TEXT,
			refreshToken TEXT,
			expiresAt TEXT,
			lastRefreshToken TEXT,
			lastRefreshTokenExpiresAt TEXT,
			createdAt TEXT,
			updatedAt TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS collections (
			id TEXT PRIMARY KEY,
			name TEXT,
			description TEXT,
			createdAt TEXT,
			updatedAt TEXT,
			libraryId TEXT,
			userId TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS collectionBooks (
			id TEXT PRIMARY KEY,
			"order" INTEGER,
			createdAt TEXT,
			bookId TEXT,
			collectionId TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS authors (
			id TEXT PRIMARY KEY,
			libraryId TEXT,
			name TEXT,
			lastFirst TEXT,
			asin TEXT,
			description TEXT,
			imagePath TEXT,
			createdAt TEXT,
			updatedAt TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS bookAuthors (
			bookId TEXT,
			authorId TEXT
		)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("failed to execute schema query %q: %v", q, err)
		}
	}

	// Insert the default server-settings record
	_, err = db.Exec(`INSERT INTO settings (key, value, createdAt, updatedAt) VALUES ('server-settings', '{"sortingIgnorePrefix": true}', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		return fmt.Errorf("failed to insert default server settings: %v", err)
	}

	return nil
}
