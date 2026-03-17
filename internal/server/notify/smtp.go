package notify

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// SMTPConfig holds the configuration for an SMTP notification channel.
type SMTPConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	To       string `json:"to"`
	TLS      bool   `json:"tls"` // true = implicit TLS (port 465) or STARTTLS (port 587)
}

const smtpDialTimeout = 10 * time.Second

// SMTPNotifier sends notifications via SMTP email.
type SMTPNotifier struct{}

// Type returns "email".
func (*SMTPNotifier) Type() string { return "email" }

// Send sends an HTML email via SMTP using the channel configuration.
// Supports STARTTLS (port 587) and implicit TLS (port 465).
func (*SMTPNotifier) Send(_ context.Context, channelConfig string, subject, body string) error {
	var cfg SMTPConfig
	if err := json.Unmarshal([]byte(channelConfig), &cfg); err != nil {
		return fmt.Errorf("parse smtp config: %w", err)
	}

	if cfg.Host == "" {
		return fmt.Errorf("smtp host is required")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return fmt.Errorf("smtp port must be between 1 and 65535")
	}
	if cfg.From == "" {
		return fmt.Errorf("smtp from address is required")
	}
	if cfg.To == "" {
		return fmt.Errorf("smtp to address is required")
	}

	for _, v := range []struct{ name, val string }{
		{"from", cfg.From}, {"to", cfg.To}, {"subject", subject},
	} {
		if strings.ContainsAny(v.val, "\r\n") {
			return fmt.Errorf("smtp %s contains invalid characters", v.name)
		}
	}

	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))

	// Build structured email body using the HTML template.
	var fields []KeyValue
	for _, line := range strings.Split(body, "\n") {
		if parts := strings.SplitN(line, ": ", 2); len(parts) == 2 {
			fields = append(fields, KeyValue{Key: parts[0], Value: parts[1]})
		}
	}
	htmlBody, renderErr := RenderEmail(EmailData{
		Subject:   subject,
		Fields:    fields,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
	if renderErr != nil {
		// Fall back to plain text body wrapped in a pre element.
		htmlBody = "<pre>" + body + "</pre>"
	}

	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s",
		cfg.From, cfg.To, subject, htmlBody,
	)

	if cfg.TLS {
		return sendWithTLS(cfg, addr, msg)
	}

	// Plain SMTP (no TLS) — suitable for local relay or internal mail servers
	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}
	return smtp.SendMail(addr, auth, cfg.From, []string{cfg.To}, []byte(msg))
}

// sendWithTLS handles both implicit TLS (port 465) and STARTTLS (port 587).
func sendWithTLS(cfg SMTPConfig, addr, msg string) error {
	tlsConfig := &tls.Config{ServerName: cfg.Host}

	var conn net.Conn
	var err error

	if cfg.Port == 465 {
		// Implicit TLS — connect directly over TLS
		conn, err = tls.DialWithDialer(&net.Dialer{Timeout: smtpDialTimeout}, "tcp", addr, tlsConfig)
	} else {
		// STARTTLS — connect plain then upgrade
		conn, err = net.DialTimeout("tcp", addr, smtpDialTimeout)
	}
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}

	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	// STARTTLS upgrade for non-465 ports
	if cfg.Port != 465 {
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}

	if cfg.Username != "" {
		auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	if err := client.Mail(cfg.From); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := client.Rcpt(cfg.To); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close data: %w", err)
	}

	return client.Quit()
}
