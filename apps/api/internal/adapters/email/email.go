// Package email sends Atlas email over SMTP: Mailpit in local development, a
// transactional provider in staging and production (§5.2, §10.1 [DECIDE]).
//
// It is the only place email is sent. The notification dispatcher calls Send
// with a deadline and treats a non-nil error as a retryable attempt, so this
// adapter must fail fast and never block indefinitely.
package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/safaristack/atlas/apps/api/internal/notifications"
)

// Config is the SMTP connection and envelope configuration.
type Config struct {
	Host     string
	Port     int
	From     string
	Username string
	Password string
	// TLS upgrades the connection with STARTTLS when the server offers it.
	TLS bool
}

// Sender is the SMTP channel adapter.
type Sender struct{ cfg Config }

// New builds a Sender, applying local-development defaults.
func New(cfg Config) *Sender {
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}
	if cfg.Port == 0 {
		cfg.Port = 1025
	}
	if cfg.From == "" {
		cfg.From = "atlas@localhost"
	}
	return &Sender{cfg: cfg}
}

// Channel reports the notification channel this adapter serves.
func (s *Sender) Channel() notifications.Channel { return notifications.ChannelEmail }

// Provider names the adapter for attempt records and metrics.
func (s *Sender) Provider() string { return "smtp" }

// Send delivers one notification message from the configured sender address.
func (s *Sender) Send(ctx context.Context, msg notifications.Message) (string, error) {
	to := strings.TrimSpace(msg.To)
	if to == "" {
		return "", fmt.Errorf("email: no recipient address")
	}
	if err := s.SendRaw(ctx, s.cfg.From, []string{to}, buildMessage(s.cfg.From, msg)); err != nil {
		return "", err
	}
	return messageID(msg.IdempotencyKey), nil
}

// SendRaw hands a fully formed RFC 5322 message to the SMTP server. from is the
// envelope sender and rcpts every envelope recipient (To, Cc and Bcc alike; Bcc
// appears only here, never in the headers). The deadline comes from ctx, or ten
// seconds, so a stalled server cannot pin a worker.
func (s *Sender) SendRaw(ctx context.Context, from string, rcpts []string, data []byte) error {
	if len(rcpts) == 0 {
		return fmt.Errorf("email: no recipient address")
	}

	deadline := time.Now().Add(10 * time.Second)
	if d, ok := ctx.Deadline(); ok {
		deadline = d
	}

	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	conn, err := net.DialTimeout("tcp", addr, time.Until(deadline))
	if err != nil {
		return fmt.Errorf("email: dial %s: %w", addr, err)
	}
	_ = conn.SetDeadline(deadline)

	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("email: smtp handshake: %w", err)
	}
	defer func() { _ = client.Quit() }()

	if s.cfg.TLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return fmt.Errorf("email: starttls: %w", err)
			}
		}
	}
	if s.cfg.Username != "" {
		auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("email: auth: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("email: mail from: %w", err)
	}
	for _, rcpt := range rcpts {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("email: rcpt to %s: %w", rcpt, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("email: data: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return fmt.Errorf("email: write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("email: close body: %w", err)
	}
	return nil
}

func buildMessage(from string, msg notifications.Message) []byte {
	var b strings.Builder
	b.WriteString("From: ")
	b.WriteString(from)
	b.WriteString("\r\nTo: ")
	b.WriteString(msg.To)
	b.WriteString("\r\nSubject: ")
	b.WriteString(sanitizeHeader(msg.Subject))
	b.WriteString("\r\n")
	if id := messageID(msg.IdempotencyKey); id != "" {
		b.WriteString("Message-ID: ")
		b.WriteString(id)
		b.WriteString("\r\n")
	}
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(msg.Body)
	b.WriteString("\r\n")
	return []byte(b.String())
}

// messageID derives a stable RFC 5322 Message-ID from the idempotency key, so a
// retried send carries the same id and a duplicate is distinguishable.
func messageID(key string) string {
	key = sanitizeHeader(key)
	if key == "" {
		return ""
	}
	return "<" + key + "@atlas.local>"
}

// sanitizeHeader removes CR/LF so a crafted subject or key cannot inject headers.
func sanitizeHeader(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(strings.TrimSpace(s))
}
