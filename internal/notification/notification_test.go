package notification

import (
	log "audiobookshelf/internal/logger"
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

var (
	testTLSConfig *tls.Config
	caFilePath    string
)

func TestMain(m *testing.M) {
	// 1. Generate certificate and write to temp file
	var err error
	testTLSConfig, caFilePath, err = createTestTLSConfig()
	if err != nil {
		log.Fatalf("failed to create test TLS config: %v", err)
	}

	// Set SSL_CERT_FILE for tests to trust the self-signed certificate
	os.Setenv("SSL_CERT_FILE", caFilePath)

	// 2. Override safeClient to bypass safeurl loopback block for WebhookNotifier tests
	safeClient = http.DefaultClient

	code := m.Run()
	os.Remove(caFilePath)
	os.Exit(code)
}

func createTestTLSConfig() (*tls.Config, string, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, "", err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "127.0.0.1",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, "", err
	}

	var certBuf bytes.Buffer
	pem.Encode(&certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	privBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, "", err
	}
	var keyBuf bytes.Buffer
	pem.Encode(&keyBuf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})

	cert, err := tls.X509KeyPair(certBuf.Bytes(), keyBuf.Bytes())
	if err != nil {
		return nil, "", err
	}

	tmpFile, err := os.CreateTemp("", "test-ca-*.crt")
	if err != nil {
		return nil, "", err
	}
	if _, err := tmpFile.Write(certBuf.Bytes()); err != nil {
		tmpFile.Close()
		return nil, "", err
	}
	tmpFile.Close()

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	return tlsConfig, tmpFile.Name(), nil
}

func handleSMTPConn(conn net.Conn, tlsConfig *tls.Config, forceTLS bool, advertiseStartTLS bool, requireAuth bool, expectedUser, expectedPass string, mailBodyChan chan<- []byte) {
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	defer conn.Close()

	if forceTLS {
		tlsConn := tls.Server(conn, tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			return
		}
		conn = tlsConn
	}

	reader := textproto.NewReader(bufio.NewReader(conn))
	writer := textproto.NewWriter(bufio.NewWriter(conn))

	writer.PrintfLine("220 localhost ESMTP MockServer")

	var authenticated bool
	if !requireAuth {
		authenticated = true
	}

	for {
		line, err := reader.ReadLine()
		if err != nil {
			return
		}

		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		cmd := strings.ToUpper(parts[0])
		switch cmd {
		case "EHLO", "HELO":
			_, isTLS := conn.(*tls.Conn)
			if !isTLS && advertiseStartTLS {
				writer.PrintfLine("250-localhost Hello")
				writer.PrintfLine("250 STARTTLS")
			} else {
				if requireAuth && !authenticated {
					writer.PrintfLine("250-localhost Hello")
					writer.PrintfLine("250 AUTH PLAIN")
				} else {
					writer.PrintfLine("250 localhost Hello")
				}
			}
		case "STARTTLS":
			_, isTLS := conn.(*tls.Conn)
			if isTLS {
				writer.PrintfLine("502 Already in TLS mode")
				continue
			}
			if !advertiseStartTLS {
				writer.PrintfLine("502 STARTTLS not supported")
				continue
			}
			writer.PrintfLine("220 2.0.0 Ready to start TLS")

			tlsConn := tls.Server(conn, tlsConfig)
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			conn = tlsConn
			reader = textproto.NewReader(bufio.NewReader(conn))
			writer = textproto.NewWriter(bufio.NewWriter(conn))

		case "AUTH":
			if len(parts) < 2 {
				writer.PrintfLine("501 Syntax error in parameters")
				continue
			}
			authType := strings.ToUpper(parts[1])
			if authType != "PLAIN" {
				writer.PrintfLine("504 Unrecognized authentication type")
				continue
			}
			var creds string
			if len(parts) >= 3 {
				creds = parts[2]
			} else {
				writer.PrintfLine("334 ")
				line, err := reader.ReadLine()
				if err != nil {
					return
				}
				creds = line
			}
			decoded, err := base64.StdEncoding.DecodeString(creds)
			if err != nil {
				writer.PrintfLine("501 Invalid base64 credentials")
				continue
			}
			authParts := bytes.Split(decoded, []byte{0})
			if len(authParts) < 3 {
				writer.PrintfLine("535 Authentication failed")
				continue
			}
			user := string(authParts[1])
			pass := string(authParts[2])
			if user == expectedUser && pass == expectedPass {
				authenticated = true
				writer.PrintfLine("235 2.7.0 Authentication successful")
			} else {
				writer.PrintfLine("535 Authentication failed")
			}

		case "MAIL":
			if requireAuth && !authenticated {
				writer.PrintfLine("530 5.7.0 Must issue a STARTTLS/AUTH command first")
				continue
			}
			writer.PrintfLine("250 2.1.0 OK")

		case "RCPT":
			if requireAuth && !authenticated {
				writer.PrintfLine("530 5.7.0 Must issue a STARTTLS/AUTH command first")
				continue
			}
			writer.PrintfLine("250 2.1.5 OK")

		case "DATA":
			if requireAuth && !authenticated {
				writer.PrintfLine("530 5.7.0 Must issue a STARTTLS/AUTH command first")
				continue
			}
			writer.PrintfLine("354 Start mail input; end with <CR><LF>.<CR><LF>")

			var body bytes.Buffer
			for {
				dotLine, err := reader.ReadLine()
				if err != nil {
					return
				}
				if dotLine == "." {
					break
				}
				if strings.HasPrefix(dotLine, ".") {
					dotLine = dotLine[1:]
				}
				body.WriteString(dotLine)
				body.WriteString("\r\n")
			}
			mailBodyChan <- body.Bytes()
			writer.PrintfLine("250 2.0.0 OK")

		case "QUIT":
			writer.PrintfLine("221 2.0.0 Bye")
			return

		default:
			writer.PrintfLine("500 Syntax error, command unrecognized")
		}
	}
}

func runMockSMTPServer(t *testing.T, forceTLS bool, advertiseStartTLS bool, requireAuth bool, expectedUser, expectedPass string, port int) (string, chan []byte, func()) {
	var l net.Listener
	var err error
	addrStr := "127.0.0.1:" + strconv.Itoa(port)
	l, err = net.Listen("tcp", addrStr)
	if err != nil {
		t.Skipf("Skipping test because unable to listen on %s: %v", addrStr, err)
	}

	addr := l.Addr().String()
	mailBodyChan := make(chan []byte, 1)
	done := make(chan struct{})

	go func() {
		defer close(done)
		conn, err := l.Accept()
		if err != nil {
			return
		}
		handleSMTPConn(conn, testTLSConfig, forceTLS, advertiseStartTLS, requireAuth, expectedUser, expectedPass, mailBodyChan)
	}()

	cleanup := func() {
		l.Close()
		<-done
	}

	return addr, mailBodyChan, cleanup
}

func verifyEmailBody(t *testing.T, emailData []byte, from string, to []string, subject string, bodyContent string) {
	parts := bytes.SplitN(emailData, []byte("\r\n\r\n"), 2)
	if len(parts) < 2 {
		t.Fatalf("invalid email format: no header/body separator found")
	}
	headers := string(parts[0])
	body := parts[1]

	if !strings.Contains(headers, fmt.Sprintf("From: %s", from)) {
		t.Errorf("missing or incorrect From header: %q", headers)
	}
	for _, rec := range to {
		if !strings.Contains(headers, rec) {
			t.Errorf("missing recipient %s in headers: %q", rec, headers)
		}
	}
	if !strings.Contains(headers, fmt.Sprintf("Subject: %s", subject)) {
		t.Errorf("missing or incorrect Subject header: %q", headers)
	}
	if !strings.Contains(headers, "MIME-Version: 1.0") {
		t.Errorf("missing MIME-Version header")
	}

	idx := strings.Index(headers, "boundary=")
	if idx == -1 {
		t.Fatalf("boundary not found in headers")
	}
	boundary := headers[idx+len("boundary="):]
	boundary = strings.Split(boundary, "\r\n")[0]
	boundary = strings.Trim(boundary, `";`)

	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	part, err := mr.NextPart()
	if err != nil {
		t.Fatalf("failed to read next multipart part: %v", err)
	}
	defer part.Close()

	if part.Header.Get("Content-Type") != "text/plain; charset=UTF-8" {
		t.Errorf("unexpected part Content-Type: %q", part.Header.Get("Content-Type"))
	}

	partBody, err := io.ReadAll(part)
	if err != nil {
		t.Fatalf("failed to read part body: %v", err)
	}

	if string(partBody) != bodyContent {
		t.Errorf("unexpected part body content:\nexpected: %q\ngot: %q", bodyContent, string(partBody))
	}
}

func TestEmailNotifier_ImplicitTLS(t *testing.T) {
	addr, bodyChan, cleanup := runMockSMTPServer(t, true, false, false, "", "", 465)
	defer cleanup()

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("failed to split host/port: %v", err)
	}
	port, _ := strconv.Atoi(portStr)

	notifier := NewEmailNotifier(host, port, "", "", "sender@example.com", []string{"receiver@example.com"})
	payload := NotificationPayload{
		Title:   "Implicit TLS Test",
		Message: "Hello via Implicit TLS!",
	}

	err = notifier.Send(context.Background(), payload)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	select {
	case body := <-bodyChan:
		verifyEmailBody(t, body, "sender@example.com", []string{"receiver@example.com"}, payload.Title, payload.Message)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for email data")
	}
}

func TestEmailNotifier_ExplicitTLS(t *testing.T) {
	addr, bodyChan, cleanup := runMockSMTPServer(t, false, true, true, "user", "pass", 0)
	defer cleanup()

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("failed to split host/port: %v", err)
	}
	port, _ := strconv.Atoi(portStr)

	notifier := NewEmailNotifier(host, port, "user", "pass", "sender@example.com", []string{"receiver1@example.com", "receiver2@example.com"})
	payload := NotificationPayload{
		Title:   "Explicit TLS Test",
		Message: "Hello via STARTTLS and Plain Auth!",
	}

	err = notifier.Send(context.Background(), payload)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	select {
	case body := <-bodyChan:
		verifyEmailBody(t, body, "sender@example.com", []string{"receiver1@example.com", "receiver2@example.com"}, payload.Title, payload.Message)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for email data")
	}
}

func TestEmailNotifier_PlainAuthNoTLS(t *testing.T) {
	addr, bodyChan, cleanup := runMockSMTPServer(t, false, false, true, "user", "pass", 0)
	defer cleanup()

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("failed to split host/port: %v", err)
	}
	port, _ := strconv.Atoi(portStr)

	notifier := NewEmailNotifier(host, port, "user", "pass", "sender@example.com", []string{"receiver@example.com"})
	payload := NotificationPayload{
		Title:   "Plain Auth No TLS Test",
		Message: "Hello via Plain Auth without TLS!",
	}

	err = notifier.Send(context.Background(), payload)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	select {
	case body := <-bodyChan:
		verifyEmailBody(t, body, "sender@example.com", []string{"receiver@example.com"}, payload.Title, payload.Message)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for email data")
	}
}

func TestEmailNotifier_NoAuthNoTLS(t *testing.T) {
	addr, bodyChan, cleanup := runMockSMTPServer(t, false, false, false, "", "", 0)
	defer cleanup()

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("failed to split host/port: %v", err)
	}
	port, _ := strconv.Atoi(portStr)

	notifier := NewEmailNotifier(host, port, "", "", "sender@example.com", []string{"receiver@example.com"})
	payload := NotificationPayload{
		Title:   "No Auth No TLS Test",
		Message: "Hello with no Auth and no TLS!",
	}

	err = notifier.Send(context.Background(), payload)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	select {
	case body := <-bodyChan:
		verifyEmailBody(t, body, "sender@example.com", []string{"receiver@example.com"}, payload.Title, payload.Message)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for email data")
	}
}

func TestEmailNotifier_ContextCancelled(t *testing.T) {
	notifier := NewEmailNotifier("127.0.0.1", 12345, "", "", "sender@example.com", []string{"receiver@example.com"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := notifier.Send(ctx, NotificationPayload{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "canceled") {
		t.Errorf("expected 'canceled' error, got: %v", err)
	}
}

func TestWebhookNotifier(t *testing.T) {
	var receivedPath string
	var receivedBody []byte
	var receivedContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedContentType = r.Header.Get("Content-Type")
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	payload := NotificationPayload{
		Title:   "Test Alert!",
		Message: "Something happened: error code 500.",
		Event:   "test_event",
		Data:    map[string]string{"foo": "bar"},
	}

	tests := []struct {
		name          string
		urlSuffix     string
		verifyPayload func(t *testing.T, body []byte)
	}{
		{
			name:      "Discord",
			urlSuffix: "/discord.com/api/webhooks/12345",
			verifyPayload: func(t *testing.T, body []byte) {
				type discordEmbed struct {
					Title       string `json:"title,omitempty"`
					Description string `json:"description,omitempty"`
					Color       int    `json:"color,omitempty"`
				}
				type discordPayload struct {
					Embeds []discordEmbed `json:"embeds"`
				}
				var dp discordPayload
				if err := json.Unmarshal(body, &dp); err != nil {
					t.Fatalf("failed to unmarshal discord body: %v", err)
				}
				if len(dp.Embeds) != 1 {
					t.Fatalf("expected 1 embed, got %d", len(dp.Embeds))
				}
				if dp.Embeds[0].Title != payload.Title {
					t.Errorf("expected title %q, got %q", payload.Title, dp.Embeds[0].Title)
				}
				if dp.Embeds[0].Description != payload.Message {
					t.Errorf("expected message %q, got %q", payload.Message, dp.Embeds[0].Description)
				}
				if dp.Embeds[0].Color != 3447003 {
					t.Errorf("expected color 3447003, got %d", dp.Embeds[0].Color)
				}
			},
		},
		{
			name:      "Telegram",
			urlSuffix: "/api.telegram.org/bot123/sendMessage",
			verifyPayload: func(t *testing.T, body []byte) {
				type telegramPayload struct {
					Text      string `json:"text"`
					ParseMode string `json:"parse_mode,omitempty"`
				}
				var tp telegramPayload
				if err := json.Unmarshal(body, &tp); err != nil {
					t.Fatalf("failed to unmarshal telegram body: %v", err)
				}
				expectedText := fmt.Sprintf("*%s*\n%s", escapeTelegramMarkdown(payload.Title), escapeTelegramMarkdown(payload.Message))
				if tp.Text != expectedText {
					t.Errorf("expected text %q, got %q", expectedText, tp.Text)
				}
				if tp.ParseMode != "MarkdownV2" {
					t.Errorf("expected parse_mode MarkdownV2, got %q", tp.ParseMode)
				}
			},
		},
		{
			name:      "Apprise",
			urlSuffix: "/notify/apprise-service",
			verifyPayload: func(t *testing.T, body []byte) {
				type apprisePayload struct {
					Title string `json:"title"`
					Body  string `json:"body"`
				}
				var ap apprisePayload
				if err := json.Unmarshal(body, &ap); err != nil {
					t.Fatalf("failed to unmarshal apprise body: %v", err)
				}
				if ap.Title != payload.Title {
					t.Errorf("expected title %q, got %q", payload.Title, ap.Title)
				}
				if ap.Body != payload.Message {
					t.Errorf("expected body %q, got %q", payload.Message, ap.Body)
				}
			},
		},
		{
			name:      "Generic",
			urlSuffix: "/generic-webhook",
			verifyPayload: func(t *testing.T, body []byte) {
				var np NotificationPayload
				if err := json.Unmarshal(body, &np); err != nil {
					t.Fatalf("failed to unmarshal generic body: %v", err)
				}
				if np.Title != payload.Title || np.Message != payload.Message || np.Event != payload.Event || np.Data["foo"] != "bar" {
					t.Errorf("payload mismatch: %+v", np)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receivedPath = ""
			receivedBody = nil
			receivedContentType = ""

			targetURL := server.URL + tt.urlSuffix
			notifier := NewWebhookNotifier(targetURL)
			err := notifier.Send(context.Background(), payload)
			if err != nil {
				t.Fatalf("Send failed: %v", err)
			}

			if receivedContentType != "application/json" {
				t.Errorf("expected application/json Content-Type, got %q", receivedContentType)
			}
			if !strings.HasSuffix(receivedPath, tt.urlSuffix) {
				t.Errorf("expected path to end with %q, got %q", tt.urlSuffix, receivedPath)
			}
			tt.verifyPayload(t, receivedBody)
		})
	}
}

func TestWebhookNotifier_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewWebhookNotifier(server.URL + "/generic")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := notifier.Send(ctx, NotificationPayload{})
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("expected 'context canceled' error, got: %v", err)
	}
}

func TestEscapeTelegramMarkdown(t *testing.T) {
	chars := []rune{'_', '*', '[', ']', '(', ')', '~', '`', '>', '#', '+', '-', '=', '|', '{', '}', '.', '!', '\\'}
	for _, r := range chars {
		escaped := escapeTelegramMarkdown(string(r))
		if escaped != "\\"+string(r) {
			t.Errorf("failed to escape %c: expected %q, got %q", r, "\\"+string(r), escaped)
		}
	}

	if escapeTelegramMarkdown("abcXYZ123") != "abcXYZ123" {
		t.Errorf("normal text got escaped: %q", escapeTelegramMarkdown("abcXYZ123"))
	}
}
