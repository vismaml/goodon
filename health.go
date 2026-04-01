package goodon

import "net/http"

// healthHandler wraps an http.Handler with a /healthz endpoint.
// /healthz is a health probe that always returns 200, indicating the gateway is running.
func healthHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		next.ServeHTTP(w, r)
	})
}
