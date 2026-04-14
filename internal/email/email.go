package email

import (
	"fmt"
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

func (s *Sender) send(to, subject, body string) error {
	msg := strings.Join([]string{
		"From: " + s.Config.From,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=\"utf-8\"",
		"",
		body,
	}, "\r\n")

	addr := s.Config.Host + ":" + s.Config.Port

	var auth smtp.Auth
	if s.Config.User != "" {
		auth = smtp.PlainAuth("", s.Config.User, s.Config.Password, s.Config.Host)
	}

	return smtp.SendMail(addr, auth, s.Config.From, []string{to}, []byte(msg))
}
