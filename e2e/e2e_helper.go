package e2e

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	_ "modernc.org/sqlite"
)

type E2EClient struct {
	Client              *http.Client
	ClientWithRedirects *http.Client
	ServerURL           string
	Jar                 *cookiejar.Jar
	Token               string
}

func NewE2EClient() *E2EClient {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar:     jar,
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	clientWithRedirects := &http.Client{
		Jar:     jar,
		Timeout: 5 * time.Second,
	}
	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:3333"
	}
	return &E2EClient{
		Client:              client,
		ClientWithRedirects: clientWithRedirects,
		ServerURL:           serverURL,
		Jar:                 jar,
	}
}

func (c *E2EClient) SetToken(token string) {
	c.Token = token
}

func (c *E2EClient) Request(method, path string, payload interface{}) (int, []byte, error) {
	return c.requestWithClient(c.Client, method, path, payload)
}

func (c *E2EClient) RequestFollowRedirects(method, path string, payload interface{}) (int, []byte, error) {
	return c.requestWithClient(c.ClientWithRedirects, method, path, payload)
}

func (c *E2EClient) requestWithClient(client *http.Client, method, path string, payload interface{}) (int, []byte, error) {
	var bodyReader io.Reader
	contentType := "application/json"
	if payload != nil {
		if s, ok := payload.(string); ok {
			bodyReader = strings.NewReader(s)
			contentType = "application/x-www-form-urlencoded"
		} else {
			b, err := json.Marshal(payload)
			if err != nil {
				return 0, nil, err
			}
			bodyReader = bytes.NewReader(b)
		}
	}

	reqURL := c.ServerURL + path
	req, err := http.NewRequest(method, reqURL, bodyReader)
	if err != nil {
		return 0, nil, err
	}

	if payload != nil {
		req.Header.Set("Content-Type", contentType)
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}

	return resp.StatusCode, respBody, nil
}

type SocketEvent struct {
	Name string
	Args []interface{}
}

type E2EWSClient struct {
	Conn *websocket.Conn
	Chan chan SocketEvent
}

func (c *E2EClient) ConnectWS(token string) (*E2EWSClient, error) {
	u, err := url.Parse(c.ServerURL)
	if err != nil {
		return nil, err
	}
	scheme := "ws"
	if u.Scheme == "https" {
		scheme = "wss"
	}
	wsURL := url.URL{
		Scheme:   scheme,
		Host:     u.Host,
		Path:     u.Path + "/socket.io/",
		RawQuery: "EIO=4&transport=websocket",
	}

	dialer := websocket.DefaultDialer
	header := http.Header{}
	if cookies := c.Jar.Cookies(u); len(cookies) > 0 {
		for _, cookie := range cookies {
			header.Add("Cookie", cookie.String())
		}
	}

	conn, _, err := dialer.Dial(wsURL.String(), header)
	if err != nil {
		return nil, err
	}

	// 1. Read Engine.io Open packet (starts with 0)
	_, msg, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return nil, err
	}
	if !strings.HasPrefix(string(msg), "0") {
		conn.Close()
		return nil, fmt.Errorf("expected Open (0), got: %s", string(msg))
	}

	// 2. Send Engine.io Connect (40)
	if err := conn.WriteMessage(websocket.TextMessage, []byte("40")); err != nil {
		conn.Close()
		return nil, err
	}

	// 3. Read Engine.io Connect response (starts with 40)
	_, msg, err = conn.ReadMessage()
	if err != nil {
		conn.Close()
		return nil, err
	}
	if !strings.HasPrefix(string(msg), "40") {
		conn.Close()
		return nil, fmt.Errorf("expected Connect (40), got: %s", string(msg))
	}

	// 4. Send Auth token (42["auth", token])
	authPayload := fmt.Sprintf(`42["auth",%q]`, token)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(authPayload)); err != nil {
		conn.Close()
		return nil, err
	}

	wsc := &E2EWSClient{
		Conn: conn,
		Chan: make(chan SocketEvent, 100),
	}

	go func() {
		defer close(wsc.Chan)
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			msgStr := string(msg)
			if strings.HasPrefix(msgStr, "42") {
				payload := msgStr[2:]
				var data []interface{}
				if err := json.Unmarshal([]byte(payload), &data); err == nil && len(data) > 0 {
					if evtName, ok := data[0].(string); ok {
						wsc.Chan <- SocketEvent{
							Name: evtName,
							Args: data[1:],
						}
					}
				}
			}
		}
	}()

	return wsc, nil
}

func (wsc *E2EWSClient) ExpectEvent(expectedName string, timeout time.Duration) (SocketEvent, error) {
	limit := time.After(timeout)
	for {
		select {
		case ev, ok := <-wsc.Chan:
			if !ok {
				return SocketEvent{}, fmt.Errorf("WebSocket read channel closed")
			}
			if ev.Name == expectedName {
				return ev, nil
			}
			// Ignore other events and keep waiting
		case <-limit:
			return SocketEvent{}, fmt.Errorf("timeout waiting for event: %s", expectedName)
		}
	}
}

func (wsc *E2EWSClient) Send(eventName string, args ...interface{}) error {
	payload := []interface{}{eventName}
	payload = append(payload, args...)
	bytesVal, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	msg := fmt.Sprintf("42%s", string(bytesVal))
	return wsc.Conn.WriteMessage(websocket.TextMessage, []byte(msg))
}

func (wsc *E2EWSClient) Close() {
	wsc.Conn.Close()
}

var CreateTableQueries = []string{
	`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT, createdAt TEXT, updatedAt TEXT)`,
	`CREATE TABLE IF NOT EXISTS users (id TEXT PRIMARY KEY, username TEXT, email TEXT, pash TEXT, type TEXT, token TEXT, isActive INTEGER, isLocked INTEGER, lastSeen INTEGER, permissions TEXT, bookmarks TEXT, extraData TEXT, createdAt TEXT, updatedAt TEXT)`,
	`CREATE TABLE IF NOT EXISTS sessions (id TEXT PRIMARY KEY, userId TEXT, ipAddress TEXT, userAgent TEXT, refreshToken TEXT, expiresAt TEXT, lastRefreshToken TEXT, lastRefreshTokenExpiresAt TEXT, createdAt TEXT, updatedAt TEXT)`,
	`CREATE TABLE IF NOT EXISTS apiKeys (id TEXT PRIMARY KEY, isActive INTEGER, expiresAt TEXT, userId TEXT)`,
	`CREATE TABLE IF NOT EXISTS libraries (id TEXT PRIMARY KEY, name TEXT, displayOrder INTEGER, icon TEXT, mediaType TEXT, provider TEXT, lastScan TEXT, lastScanVersion TEXT, settings TEXT, createdAt TEXT, updatedAt TEXT)`,
	`CREATE TABLE IF NOT EXISTS libraryFolders (id TEXT PRIMARY KEY, path TEXT, libraryId TEXT, createdAt TEXT, updatedAt TEXT)`,
	`CREATE TABLE IF NOT EXISTS libraryItems (id TEXT PRIMARY KEY, ino TEXT, libraryId TEXT, path TEXT, relPath TEXT, isFile INTEGER, mtime TEXT, ctime TEXT, birthtime TEXT, createdAt TEXT, updatedAt TEXT, isMissing INTEGER, isInvalid INTEGER, mediaType TEXT, mediaId TEXT, size INTEGER, libraryFolderId TEXT, authorNamesFirstLast TEXT, authorNamesLastFirst TEXT, title TEXT, titleIgnorePrefix TEXT)`,
	`CREATE TABLE IF NOT EXISTS books (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT, subtitle TEXT, publishedYear TEXT, publishedDate TEXT, publisher TEXT, description TEXT, isbn TEXT, asin TEXT, language TEXT, explicit INTEGER, abridged INTEGER, coverPath TEXT, duration REAL, narrators BLOB, audioFiles BLOB, ebookFile BLOB, chapters BLOB, tags BLOB, genres BLOB)`,
	`CREATE TABLE IF NOT EXISTS podcasts (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT, author TEXT, releaseDate TEXT, feedURL TEXT, imageURL TEXT, description TEXT, itunesPageURL TEXT, itunesId TEXT, itunesArtistId TEXT, language TEXT, podcastType TEXT, explicit INTEGER, autoDownloadEpisodes INTEGER, autoDownloadSchedule TEXT, lastEpisodeCheck TEXT, maxEpisodesToKeep INTEGER, maxNewEpisodesToDownload INTEGER, coverPath TEXT, tags BLOB, genres BLOB, numEpisodes INTEGER)`,
	`CREATE TABLE IF NOT EXISTS bookSeries (bookId TEXT, seriesId TEXT, sequence TEXT)`,
	`CREATE TABLE IF NOT EXISTS series (id TEXT PRIMARY KEY, name TEXT)`,
	`CREATE TABLE IF NOT EXISTS mediaProgresses (id TEXT PRIMARY KEY, userId TEXT, mediaItemId TEXT, mediaItemType TEXT, duration REAL, currentTime REAL, isFinished INTEGER, hideFromContinueListening INTEGER, ebookLocation TEXT, ebookProgress REAL, finishedAt TEXT, extraData TEXT, podcastId TEXT, createdAt TEXT, updatedAt TEXT)`,
	`CREATE TABLE IF NOT EXISTS playbackSessions (id TEXT PRIMARY KEY, userId TEXT, mediaItemId TEXT, mediaItemType TEXT, startTime REAL, libraryId TEXT, extraData TEXT)`,
	`CREATE TABLE IF NOT EXISTS podcastEpisodes (id TEXT PRIMARY KEY, podcastId TEXT, title TEXT, audioFile TEXT)`,
	`CREATE TABLE IF NOT EXISTS playlists (id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT, createdAt TEXT, updatedAt TEXT, libraryId TEXT, userId TEXT)`,
	`CREATE TABLE IF NOT EXISTS playlistMediaItems (id TEXT PRIMARY KEY, mediaItemId TEXT, mediaItemType TEXT, "order" INTEGER, createdAt TEXT, playlistId TEXT)`,
	`CREATE TABLE IF NOT EXISTS customMetadataProviders (id TEXT PRIMARY KEY, name TEXT, mediaType TEXT, url TEXT, authHeaderValue TEXT, extraData TEXT, createdAt INTEGER, updatedAt INTEGER)`,
	`CREATE TABLE IF NOT EXISTS authors (id TEXT PRIMARY KEY, name TEXT, lastFirst TEXT, asin TEXT, description TEXT, imagePath TEXT, createdAt TEXT, updatedAt TEXT, libraryId TEXT)`,
	`CREATE TABLE IF NOT EXISTS bookAuthors (bookId TEXT, authorId TEXT)`,
}

func InitializeOrResetDatabase() error {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		return fmt.Errorf("CONFIG_PATH is not set, cannot locate or reset database")
	}

	if err := os.MkdirAll(configPath, 0755); err != nil {
		return fmt.Errorf("failed to create config directory %s: %v", configPath, err)
	}

	dbPath := filepath.Join(configPath, "absdatabase.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open sqlite DB at %s: %v", dbPath, err)
	}
	defer db.Close()

	for _, q := range CreateTableQueries {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("failed to run query %q: %v", q, err)
		}
	}

	tables := []string{
		"settings", "users", "sessions", "apiKeys", "libraries", "libraryFolders",
		"libraryItems", "books", "podcasts", "bookSeries", "series",
		"mediaProgresses", "playbackSessions", "podcastEpisodes", "playlists", "playlistMediaItems",
		"customMetadataProviders", "authors", "bookAuthors",
	}
	for _, table := range tables {
		if _, err := db.Exec("DELETE FROM " + table); err != nil {
			return fmt.Errorf("failed to truncate table %s: %v", table, err)
		}
	}

	_, err = db.Exec(`INSERT INTO settings (key, value, createdAt, updatedAt) VALUES ('server-settings', '{"sortingIgnorePrefix": true}', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		return fmt.Errorf("failed to insert default server settings: %v", err)
	}

	return nil
}
