package web

import "net/http"

// ContentSecurityPolicy returns a middleware that sets a strict
// Content-Security-Policy header on every response. All resources
// (scripts, styles, images) are loaded from the same origin ('self').
// Inline scripts and styles are permitted for HTMX config, theme
// switching, and Tailwind utility classes.
func ContentSecurityPolicy(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-inline'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data:; "+
				"font-src 'self'; "+
				"connect-src 'self'; "+
				"frame-src 'none'; "+
				"object-src 'none'; "+
				"base-uri 'self'; "+
				"form-action 'self'",
		)
		next.ServeHTTP(w, r)
	})
}
