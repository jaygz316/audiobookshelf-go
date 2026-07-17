package handlers

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime/multipart"
	"net"
	"net/smtp"
	"net/textproto"
	"os"
	"strconv"
)

// sendMail connects to the SMTP server and sends a raw MIME email (with optional attachment)
func sendMail(ctx context.Context, host string, port int, secure bool, rejectUnauthorized bool, user, pass, fromAddress, toAddress, subject, body string, attachmentPath string, attachmentName string) error {
	addr := net.JoinHostPort(host, strconv.Itoa(port))

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}

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

	tlsConfig := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: !rejectUnauthorized,
	}

	if secure || port == 465 {
		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("SMTP TLS handshake failed: %w", err)
		}
		conn = tlsConn
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	if !secure && port != 465 {
		if hasStartTLS, _ := client.Extension("STARTTLS"); hasStartTLS {
			if err = client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("SMTP STARTTLS failed: %w", err)
			}
		}
	}

	if user != "" || pass != "" {
		auth := smtp.PlainAuth("", user, pass, host)
		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP auth failed: %w", err)
		}
	}

	if err = client.Mail(fromAddress); err != nil {
		return fmt.Errorf("failed to set SMTP MAIL FROM: %w", err)
	}

	if err = client.Rcpt(toAddress); err != nil {
		return fmt.Errorf("failed to set SMTP RCPT TO (%s): %w", toAddress, err)
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

	buf := new(bytes.Buffer)
	buf.WriteString(fmt.Sprintf("From: %s\r\n", fromAddress))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", toAddress))
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	buf.WriteString("MIME-Version: 1.0\r\n")

	writer := multipart.NewWriter(buf)
	if attachmentPath != "" {
		buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=%s\r\n\r\n", writer.Boundary()))

		// Text part
		if body != "" {
			partHeader := make(textproto.MIMEHeader)
			partHeader.Set("Content-Type", "text/plain; charset=UTF-8")
			part, err := writer.CreatePart(partHeader)
			if err != nil {
				return fmt.Errorf("failed to create MIME text part: %w", err)
			}
			if _, err = part.Write([]byte(body)); err != nil {
				return fmt.Errorf("failed to write body to MIME part: %w", err)
			}
		}

		// Attachment part
		attachmentBytes, err := os.ReadFile(attachmentPath)
		if err != nil {
			return fmt.Errorf("failed to read attachment file: %w", err)
		}

		partHeader := make(textproto.MIMEHeader)
		partHeader.Set("Content-Type", "application/octet-stream")
		partHeader.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, attachmentName))
		partHeader.Set("Content-Transfer-Encoding", "base64")
		part, err := writer.CreatePart(partHeader)
		if err != nil {
			return fmt.Errorf("failed to create MIME attachment part: %w", err)
		}

		b64 := base64.StdEncoding.EncodeToString(attachmentBytes)
		if _, err = part.Write([]byte(b64)); err != nil {
			return fmt.Errorf("failed to write attachment data to MIME part: %w", err)
		}
	} else {
		buf.WriteString(fmt.Sprintf("Content-Type: text/plain; charset=UTF-8\r\n\r\n%s", body))
	}

	if attachmentPath != "" {
		if err = writer.Close(); err != nil {
			return fmt.Errorf("failed to close MIME writer: %w", err)
		}
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
