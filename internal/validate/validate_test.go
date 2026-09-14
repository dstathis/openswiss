package validate

import "strings"

import "testing"

func TestDisplayName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"ok", "Alice", ""},
		{"empty", "", "Display name is required."},
		{"at limit", strings.Repeat("a", MaxDisplayNameLen), ""},
		{"over limit", strings.Repeat("a", MaxDisplayNameLen+1), "Display name must be 100 characters or fewer."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, ok := DisplayName(tt.input)
			if ok != (tt.want == "") || msg != tt.want {
				t.Errorf("DisplayName(%d chars) = (%q, %v), want (%q, %v)", len(tt.input), msg, ok, tt.want, tt.want == "")
			}
		})
	}
}

func TestNewPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		confirm  string
		want     string
	}{
		{"ok", "hunter2hunter2", "hunter2hunter2", ""},
		{"at limit", "12345678", "12345678", ""},
		{"too short", "1234567", "1234567", "Password must be at least 8 characters."},
		{"mismatch", "12345678", "12345679", "Passwords do not match."},
		// Length is checked before the match, so a short password reports its
		// length rather than confusing the user with a mismatch message.
		{"short and mismatched", "123", "1234567890", "Password must be at least 8 characters."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, ok := NewPassword(tt.password, tt.confirm)
			if ok != (tt.want == "") || msg != tt.want {
				t.Errorf("NewPassword() = (%q, %v), want (%q, %v)", msg, ok, tt.want, tt.want == "")
			}
		})
	}
}

func TestEmail(t *testing.T) {
	long := strings.Repeat("a", MaxEmailLen) + "@example.com"
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"ok", "player@example.com", ""},
		{"no at sign", "player.example.com", "Please enter a valid email address."},
		{"no domain dot", "player@example", "Please enter a valid email address."},
		{"empty", "", "Please enter a valid email address."},
		{"whitespace", "pla yer@example.com", "Please enter a valid email address."},
		{"too long", long, "Email address is too long."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, ok := Email(tt.input)
			if ok != (tt.want == "") || msg != tt.want {
				t.Errorf("Email(%q) = (%q, %v), want (%q, %v)", tt.input, msg, ok, tt.want, tt.want == "")
			}
		})
	}
}
