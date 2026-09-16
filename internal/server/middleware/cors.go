package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"

	khttp "github.com/go-kratos/kratos/v3/transport/http"
)

// CORS returns a filter that answers preflight requests and adds the
// configured CORS headers to every reply. It runs outside the router so an
// OPTIONS request is answered even though no route is registered for it.
func CORS(origins, methods, headers []string, allowCredentials bool, maxAge int) khttp.FilterFunc {
	originSet := make(map[string]struct{}, len(origins))
	allowAll := false
	for _, o := range origins {
		o = strings.TrimSpace(o)
		if o == "*" {
			allowAll = true
		}
		if o != "" {
			originSet[o] = struct{}{}
		}
	}
	if len(methods) == 0 {
		methods = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions}
	}
	if len(headers) == 0 {
		headers = []string{"Authorization", "Content-Type", "X-Node-Token", "X-Request-Id", "X-Share-Token", "Accept", "Origin"}
	}
	allowMethods := strings.Join(methods, ", ")
	allowHeaders := strings.Join(headers, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if _, ok := originSet[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Add("Vary", "Origin")
				} else if allowAll && !allowCredentials {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				}
				if allowCredentials {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				w.Header().Set("Access-Control-Allow-Methods", allowMethods)
				w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
				w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition, Content-Length, Content-Range, ETag, X-Request-Id")
				if maxAge > 0 {
					w.Header().Set("Access-Control-Max-Age", strconv.Itoa(maxAge))
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequestID propagates or mints a correlation id for every request.
func RequestID() khttp.FilterFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-Id")
			if id == "" {
				id = newRequestID()
			}
			r.Header.Set("X-Request-Id", id)
			w.Header().Set("X-Request-Id", id)
			next.ServeHTTP(w, r)
		})
	}
}

// newRequestID returns a short random correlation id.
func newRequestID() string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "req-unknown"
	}
	return hex.EncodeToString(buf[:])
}
