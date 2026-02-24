package handler

import (
	"log"
	"net/http"
	"time"
)

// statusRecorder wraps http.ResponseWriter to capture the written status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// withMiddleware wraps the given handler with: body size limit, panic recovery,
// and structured request logging (method, path, status, duration).
func (h *Handler) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()

		defer func() {
			if p := recover(); p != nil {
				log.Printf("panic recovered: %v", p)
				writeError(rec, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
			}
			log.Printf("request method=%s path=%s remote=%s status=%d duration=%s",
				r.Method, r.URL.Path, r.RemoteAddr, rec.status, time.Since(start))
		}()

		max := h.cfg.MaxBodyBytes
		if max <= 0 {
			max = 1 << 20
		}
		// Fast rejection: Content-Length header already exceeds limit.
		if r.ContentLength > max {
			writeError(rec, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body too large")
			return
		}
		// Always wrap body so chunked/unknown-length bodies are also capped.
		r.Body = http.MaxBytesReader(rec, r.Body, max)

		next.ServeHTTP(rec, r)
	})
}
