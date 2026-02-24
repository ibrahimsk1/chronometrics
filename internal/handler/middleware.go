package handler

import (
	"log"
	"net/http"
)

// withMiddleware wraps the given handler with common middleware: body limit (for requests with body),
// panic recovery, and simple structured logging.
func (h *Handler) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Panic recovery
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic recovered: %v", rec)
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
			}
		}()

		// Body size check for requests with bodies
		if r.ContentLength > 0 {
			max := h.cfg.MaxBodyBytes
			if max <= 0 {
				max = 1 << 20
			}
			if r.ContentLength > max {
				writeError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body too large")
				return
			}
			// also ensure we won't read more than max downstream
			r.Body = http.MaxBytesReader(w, r.Body, max)
		}

		// Simple structured request log
		log.Printf("request method=%s path=%s remote=%s", r.Method, r.URL.Path, r.RemoteAddr)

		// Call next
		next.ServeHTTP(w, r)
	})
}
