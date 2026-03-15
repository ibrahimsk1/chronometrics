package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
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

type requestIDKey struct{}

// withMiddleware wraps the given handler with: body size limit, panic recovery,
// and structured request logging (method, path, status, duration).
func (h *Handler) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()

		defer func() {
			if p := recover(); p != nil {
				h.logger.Error("panic recovered", "panic", p)
				writeError(rec, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
			}
			elapsed := time.Since(start)
			h.logger.Info("request",
				"request_id", w.Header().Get("X-Request-ID"),
				"method", r.Method,
				"path", r.URL.Path,
				"remote", r.RemoteAddr,
				"status", rec.status,
				"duration_ms", elapsed.Milliseconds(),
			)
			if h.metrics != nil {
				h.metrics.HTTPRequestsTotal.WithLabelValues(r.Method, r.URL.Path, strconv.Itoa(rec.status)).Inc()
				h.metrics.HTTPRequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(elapsed.Seconds())
			}
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

		requestId := uuid.NewString()
		w.Header().Set("X-Request-ID", requestId)
		ctx := context.WithValue(r.Context(), requestIDKey{}, requestId)
		r = r.WithContext(ctx)

		next.ServeHTTP(rec, r)
	})
}

func (h *Handler) withLimiter(name string, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rl, ok := h.limiters[name]
		if !ok || rl == nil {
			next.ServeHTTP(w, r)
			return
		}

		rl.mu.Lock()
		if rl.count >= rl.limit {
			rl.mu.Unlock()
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "rate limit exceeded")
			return
		}
		rl.count++
		rl.mu.Unlock()

		next.ServeHTTP(w, r)
	})
}
