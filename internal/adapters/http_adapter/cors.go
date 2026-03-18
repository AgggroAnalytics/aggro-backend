package httpadapter

import (
	"net/http"
	"strings"
)

// CORS wraps the handler: handles OPTIONS, sets Allow-* for browser SPA.
func CORS(allowOrigins string, next http.Handler) http.Handler {
	origins := map[string]struct{}{}
	for _, o := range strings.Split(allowOrigins, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			origins[o] = struct{}{}
		}
	}
	if len(origins) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if _, ok := origins[origin]; ok {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		allowHdr := "Authorization, Content-Type, Accept, X-Requested-With"
		if reqH := r.Header.Get("Access-Control-Request-Headers"); reqH != "" {
			allowHdr = reqH
		}
		w.Header().Set("Access-Control-Allow-Headers", allowHdr)
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
