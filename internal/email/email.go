package email

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

// Config holds SMTP configuration.
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	From     string
}

// Enabled returns true if SMTP is configured.
func (c *Config) Enabled() bool {
	return c.Host != "" && c.From != ""
}

// Sender sends emails via SMTP.
type Sender struct {
	Config Config
}

// SendPasswordReset sends a password-reset email with the given token link.
func (s *Sender) SendPasswordReset(to, resetURL string) error {
	subject := "OpenSwiss — Password Reset"
	body := fmt.Sprintf(
		"You requested a password reset for your OpenSwiss account.\n\n"+
			"Click the link below to reset your password (valid for 1 hour):\n\n"+
			"%s\n\n"+
			"If you did not request this, you can ignore this email.",
		resetURL,
	)

	return s.send(to, subject, body)
}

func (s *Sender) buildMessage(to, subject, body string) []byte {
	msg := strings.Join([]string{
		"From: " + s.Config.From,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=\"utf-8\"",
		"",
		body,
	}, "\r\n")
	return []byte(msg)
}

func (s *Sender) send(to, subject, body string) error {
	addr := net.JoinHostPort(s.Config.Host, s.Config.Port)
	msg := s.buildMessage(to, subject, body)

	// Port 465 uses implicit TLS (SMTPS), other ports use STARTTLS.
	if s.Config.Port == "465" {
		return s.sendImplicitTLS(addr, to, msg)
	}
	return s.sendSTARTTLS(addr, to, msg)
}

// sendSTARTTLS connects in plaintext and upgrades via STARTTLS (port 587/25).
func (s *Sender) sendSTARTTLS(addr, to string, msg []byte) error {
	var auth smtp.Auth
	if s.Config.User != "" {
		auth = smtp.PlainAuth("", s.Config.User, s.Config.Password, s.Config.Host)
	}
	return smtp.SendMail(addr, auth, s.Config.From, []string{to}, msg)
}

// sendImplicitTLS connects over TLS from the start (port 465).
func (s *Sender) sendImplicitTLS(addr, to string, msg []byte) error {
	tlsConfig := &tls.Config{ServerName: s.Config.Host}
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, s.Config.Host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	if s.Config.User != "" {
		auth := smtp.PlainAuth("", s.Config.User, s.Config.Password, s.Config.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	if err := client.Mail(s.Config.From); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close data: %w", err)
	}
	return client.Quit()
}
