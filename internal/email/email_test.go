package email

import (
	"strings"
	"testing"
)

func TestConfig_Enabled(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected bool
	}{
		{"fully configured", Config{Host: "smtp.example.com", Port: "587", User: "user", Password: "pass", From: "noreply@example.com"}, true},
		{"host and from only", Config{Host: "smtp.example.com", From: "noreply@example.com"}, true},
		{"missing host", Config{Port: "587", From: "noreply@example.com"}, false},
		{"missing from", Config{Host: "smtp.example.com", Port: "587"}, false},
		{"empty config", Config{}, false},
		{"only host", Config{Host: "smtp.example.com"}, false},
		{"only from", Config{From: "noreply@example.com"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.Enabled(); got != tt.expected {
				t.Errorf("Config.Enabled() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSender_BuildMessage(t *testing.T) {
	s := &Sender{Config: Config{From: "noreply@example.com"}}
	msg := s.buildMessage("user@example.com", "Test Subject", "Hello")
	str := string(msg)

	if !strings.Contains(str, "From: noreply@example.com") {
		t.Error("expected From header")
	}
	if !strings.Contains(str, "To: user@example.com") {
		t.Error("expected To header")
	}
	if !strings.Contains(str, "Subject: Test Subject") {
		t.Error("expected Subject header")
	}
	if !strings.Contains(str, "Hello") {
		t.Error("expected body")
	}
}

func TestSender_Send_RoutesImplicitTLS(t *testing.T) {
	// Port 465 should attempt implicit TLS (will fail to connect, but verifies routing)
	s := &Sender{Config: Config{
		Host: "127.0.0.1",
		Port: "465",
		From: "noreply@example.com",
	}}
	err := s.send("user@example.com", "Test", "Body")
	if err == nil {
		t.Fatal("expected connection error to 127.0.0.1:465")
	}
	if !strings.Contains(err.Error(), "tls dial") {
		t.Errorf("expected tls dial error for port 465, got: %v", err)
	}
}

func TestSender_Send_RoutesSTARTTLS(t *testing.T) {
	// Port 587 should attempt STARTTLS via smtp.SendMail
	s := &Sender{Config: Config{
		Host: "127.0.0.1",
		Port: "587",
		From: "noreply@example.com",
	}}
	err := s.send("user@example.com", "Test", "Body")
	if err == nil {
		t.Fatal("expected connection error to 127.0.0.1:587")
	}
	// smtp.SendMail returns a dial error, not a "tls dial" error
	if strings.Contains(err.Error(), "tls dial") {
		t.Errorf("port 587 should not use implicit TLS, got: %v", err)
	}
}
