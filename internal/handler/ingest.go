package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"eventmetrics/internal/domain"
)
func (h *Handler) handleIngest(w http.ResponseWriter, r *http.Request) {
	// Only allow POST
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	ctx := r.Context()
	// enforce body limit
	max := h.cfg.MaxBodyBytes
	if max <= 0 {
		max = 1 << 20 // 1MB default
	}
	r.Body = http.MaxBytesReader(w, r.Body, max)
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body too large")
		return
	}
	var ev domain.RawEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON")
		return
	}
	// Delegate validation + normalization to use-case.
	if err := h.ingester.Ingest(ctx, &ev); err != nil {
		// Validation errors -> 400
		if domain.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
		// Publish failures -> 503 (service overloaded)
		if errors.Is(err, domain.ErrPublishFailed) {
			// Best-effort Retry-After header per TDD (nats: 1s).
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusServiceUnavailable, "PUBLISH_FAILED", "service overloaded, retry later")
			return
		}
		// Unknown/internal errors -> 500
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	// accepted
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (h *Handler) handleIngestBulk(w http.ResponseWriter, r *http.Request) {
	// bulk handler previously stubbed here; implementation lives in bulk.go
	writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "bulk ingestion handled by bulk.go")
}
