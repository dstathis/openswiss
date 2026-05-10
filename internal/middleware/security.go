package middleware

import "net/http"

// contentSecurityPolicy is strict by design: no inline scripts or styles, no
// eval, no third-party origins. Every script the app needs is in
// /static/app.js; every stylesheet is /static/style.css; fonts and image
// data URLs are served same-origin (or embedded as data: URIs).
//
// Adding new third-party assets, inline scripts, or onclick attributes will
// be silently blocked by the browser — that's the point. If you need
// something here, prefer hosting it under /static/ rather than relaxing the
// policy.
const contentSecurityPolicy = "default-src 'self'; " +
	"img-src 'self' data:; " +
	"font-src 'self'; " +
	"style-src 'self'; " +
	"script-src 'self'; " +
	"form-action 'self'; " +
	"frame-ancestors 'none'; " +
	"base-uri 'self'; " +
	"object-src 'none'"

// SecureHeaders adds security-related HTTP headers to all responses.
//
// hsts controls whether to emit Strict-Transport-Security. Only enable it when
// the app is actually served over HTTPS — otherwise browsers will refuse the
// HTTP scheme on subsequent visits and you've effectively bricked the site.
func SecureHeaders(hsts bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			h.Set("Content-Security-Policy", contentSecurityPolicy)
			if hsts {
				// 1 year, include subdomains. preload omitted intentionally —
				// preload is a one-way submission and shouldn't be turned on
				// without an explicit decision.
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}
