package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type realIPKey struct{}

// RealIP resolves the client's IP address and stores it on the request context.
// Downstream middleware should call ClientIP(r) instead of touching r.RemoteAddr
// or r.Header.Get("X-Forwarded-For") directly.
//
// X-Forwarded-For / X-Real-IP are honored only when the immediate connection
// comes from one of the trustedProxies CIDRs. Without that gate any client
// could spoof the header to evade per-IP rate limits or bypass IP-based bans.
//
// trustedProxies is typically the docker/k8s pod CIDR and any L7 load balancer
// subnet in front of the app. If empty, the header is ignored entirely and the
// connection's RemoteAddr is used.
func RealIP(trustedProxies []*net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := remoteIP(r)
			if ip != "" && isTrustedProxy(ip, trustedProxies) {
				if forwarded := firstForwardedFor(r.Header.Get("X-Forwarded-For")); forwarded != "" {
					ip = forwarded
				} else if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
					ip = real
				}
			}
			ctx := context.WithValue(r.Context(), realIPKey{}, ip)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClientIP returns the resolved client IP, falling back to RemoteAddr if RealIP
// middleware is not in the chain.
func ClientIP(r *http.Request) string {
	if ip, ok := r.Context().Value(realIPKey{}).(string); ok && ip != "" {
		return ip
	}
	return remoteIP(r)
}

// ParseTrustedProxies parses a comma-separated list of CIDR blocks. Bare IPs
// are accepted and treated as /32 or /128.
func ParseTrustedProxies(spec string) ([]*net.IPNet, error) {
	if spec = strings.TrimSpace(spec); spec == "" {
		return nil, nil
	}
	var out []*net.IPNet
	for _, raw := range strings.Split(spec, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !strings.Contains(raw, "/") {
			if ip := net.ParseIP(raw); ip != nil {
				if ip.To4() != nil {
					raw += "/32"
				} else {
					raw += "/128"
				}
			}
		}
		_, cidr, err := net.ParseCIDR(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, cidr)
	}
	return out, nil
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func firstForwardedFor(header string) string {
	if header == "" {
		return ""
	}
	first, _, _ := strings.Cut(header, ",")
	return strings.TrimSpace(first)
}

func isTrustedProxy(ip string, trusted []*net.IPNet) bool {
	if len(trusted) == 0 {
		return false
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, cidr := range trusted {
		if cidr.Contains(parsed) {
			return true
		}
	}
	return false
}
