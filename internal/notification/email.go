package notification

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"mime/multipart"
	"net"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
)

// EmailNotifier connects to an SMTP server to send email notifications.
type EmailNotifier struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	To       []string
}

// NewEmailNotifier creates a new EmailNotifier.
func NewEmailNotifier(host string, port int, user, pass, from string, to []string) *EmailNotifier {
	return &EmailNotifier{
		Host:     host,
		Port:     port,
		Username: user,
		Password: pass,
		From:     from,
		To:       to,
	}
}

// Send dispatches the notification payload as a raw multipart MIME email.
func (n *EmailNotifier) Send(ctx context.Context, payload NotificationPayload) error {
	addr := net.JoinHostPort(n.Host, strconv.Itoa(n.Port))

	// PORT: Use net.Dialer with Context to respect the cancellation signal.
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()

	// Set connection deadline if context has one.
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}

	// Close connection on context cancellation to unblock pending I/O.
	if ctx.Done() != nil {
		doneChan := make(chan struct{})
		defer close(doneChan)
		go func() {
			select {
			case <-ctx.Done():
				conn.Close()
			case <-doneChan:
			}
		}()
	}

	// PORT: Perform implicit TLS handshake if connecting to standard SMTPS port 465.
	if n.Port == 465 {
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName: n.Host,
		})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("SMTP TLS handshake failed: %w", err)
		}
		conn = tlsConn
	}

	client, err := smtp.NewClient(conn, n.Host)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	// PORT: Support explicit TLS upgrade via STARTTLS for non-465 ports if the server supports it.
	if n.Port != 465 {
		if hasStartTLS, _ := client.Extension("STARTTLS"); hasStartTLS {
			tlsConfig := &tls.Config{
				ServerName: n.Host,
			}
			if err = client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("SMTP STARTTLS failed: %w", err)
			}
		}
	}

	// PORT: Perform SMTP plain auth if username/password credentials are configured.
	if n.Username != "" || n.Password != "" {
		auth := smtp.PlainAuth("", n.Username, n.Password, n.Host)
		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP auth failed: %w", err)
		}
	}

	if err = client.Mail(n.From); err != nil {
		return fmt.Errorf("failed to set SMTP MAIL FROM: %w", err)
	}

	for _, to := range n.To {
		if err = client.Rcpt(to); err != nil {
			return fmt.Errorf("failed to set SMTP RCPT TO (%s): %w", to, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to open SMTP data writer: %w", err)
	}
	var writerClosed bool
	defer func() {
		if !writerClosed {
			w.Close()
		}
	}()

	// PORT: Construct a clean raw multipart MIME email body.
	buf := new(bytes.Buffer)
	buf.WriteString(fmt.Sprintf("From: %s\r\n", n.From))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(n.To, ", ")))
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", payload.Title))
	buf.WriteString("MIME-Version: 1.0\r\n")

	writer := multipart.NewWriter(buf)
	buf.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=%s\r\n\r\n", writer.Boundary()))

	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Type", "text/plain; charset=UTF-8")
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return fmt.Errorf("failed to create MIME part: %w", err)
	}

	if _, err = part.Write([]byte(payload.Message)); err != nil {
		return fmt.Errorf("failed to write message to MIME part: %w", err)
	}

	if err = writer.Close(); err != nil {
		return fmt.Errorf("failed to close MIME writer: %w", err)
	}

	if _, err = w.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("failed to write raw email body: %w", err)
	}

	writerClosed = true
	if err = w.Close(); err != nil {
		return fmt.Errorf("failed to close SMTP data writer: %w", err)
	}

	_ = client.Quit()

	return nil
}
