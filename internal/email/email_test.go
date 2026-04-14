package email

import (
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
