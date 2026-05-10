package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRealIP_NoTrustedProxies_IgnoresXForwardedFor(t *testing.T) {
	trusted, err := ParseTrustedProxies("")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var got string
	h := RealIP(trusted)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = ClientIP(r)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.5:1234"
	req.Header.Set("X-Forwarded-For", "evil.example, 10.0.0.1")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != "203.0.113.5" {
		t.Errorf("expected RemoteAddr (203.0.113.5), got %q", got)
	}
}

func TestRealIP_TrustedProxy_HonorsXForwardedFor(t *testing.T) {
	trusted, err := ParseTrustedProxies("172.16.0.0/12")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var got string
	h := RealIP(trusted)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = ClientIP(r)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "172.18.0.5:1234" // pretend Caddy in docker bridge net
	req.Header.Set("X-Forwarded-For", "203.0.113.99, 172.18.0.5")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != "203.0.113.99" {
		t.Errorf("expected leftmost XFF (203.0.113.99), got %q", got)
	}
}

func TestRealIP_UntrustedRemoteAddr_IgnoresXForwardedFor(t *testing.T) {
	trusted, err := ParseTrustedProxies("172.16.0.0/12")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var got string
	h := RealIP(trusted)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = ClientIP(r)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "8.8.8.8:1234" // not in trusted CIDR
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != "8.8.8.8" {
		t.Errorf("expected RemoteAddr (8.8.8.8), got %q", got)
	}
}

func TestParseTrustedProxies(t *testing.T) {
	cases := []struct {
		spec   string
		count  int
		hasErr bool
	}{
		{"", 0, false},
		{"10.0.0.0/8", 1, false},
		{"10.0.0.0/8, 172.16.0.0/12", 2, false},
		{"127.0.0.1", 1, false},
		{"::1", 1, false},
		{"not-an-ip", 0, true},
	}
	for _, c := range cases {
		got, err := ParseTrustedProxies(c.spec)
		if c.hasErr && err == nil {
			t.Errorf("%q: expected error", c.spec)
			continue
		}
		if !c.hasErr && err != nil {
			t.Errorf("%q: unexpected error: %v", c.spec, err)
			continue
		}
		if len(got) != c.count {
			t.Errorf("%q: expected %d cidrs, got %d", c.spec, c.count, len(got))
		}
	}
}
