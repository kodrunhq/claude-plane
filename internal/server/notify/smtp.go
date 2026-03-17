package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/smtp"
)

// SMTPConfig holds the configuration for an SMTP notification channel.
type SMTPConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	To       string `json:"to"`
}

// SMTPNotifier sends notifications via SMTP email.
type SMTPNotifier struct{}

// Type returns "email".
func (*SMTPNotifier) Type() string { return "email" }

// Send sends an HTML email via SMTP using the channel configuration.
func (*SMTPNotifier) Send(_ context.Context, channelConfig string, subject, body string) error {
	var cfg SMTPConfig
	if err := json.Unmarshal([]byte(channelConfig), &cfg); err != nil {
		return fmt.Errorf("parse smtp config: %w", err)
	}

	if cfg.Host == "" {
		return fmt.Errorf("smtp host is required")
	}
	if cfg.From == "" {
		return fmt.Errorf("smtp from address is required")
	}
	if cfg.To == "" {
		return fmt.Errorf("smtp to address is required")
	}

	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))

	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s",
		cfg.From, cfg.To, subject, body,
	)

	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}

	return smtp.SendMail(addr, auth, cfg.From, []string{cfg.To}, []byte(msg))
}
