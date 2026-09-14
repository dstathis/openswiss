// Package validate holds the account field rules shared by the web forms and
// the JSON API, so registration, account settings and PATCH /users/me can't
// drift apart on what counts as an acceptable name, password or address.
package validate

import "regexp"

// EmailRegexp is deliberately permissive: it rejects obvious typos without
// trying to out-guess RFC 5322. Deliverability is proven by the verification
// email, not by this pattern.
var EmailRegexp = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

const (
	MaxDisplayNameLen = 100
	MaxEmailLen       = 254
	MinPasswordLen    = 8
)

// Each validator returns a user-facing message and false when the value is
// unacceptable, and ("", true) when it passes.

func DisplayName(displayName string) (string, bool) {
	if displayName == "" {
		return "Display name is required.", false
	}
	if len(displayName) > MaxDisplayNameLen {
		return "Display name must be 100 characters or fewer.", false
	}
	return "", true
}

func NewPassword(password, confirmPassword string) (string, bool) {
	if len(password) < MinPasswordLen {
		return "Password must be at least 8 characters.", false
	}
	if password != confirmPassword {
		return "Passwords do not match.", false
	}
	return "", true
}

func Email(addr string) (string, bool) {
	if !EmailRegexp.MatchString(addr) {
		return "Please enter a valid email address.", false
	}
	if len(addr) > MaxEmailLen {
		return "Email address is too long.", false
	}
	return "", true
}
