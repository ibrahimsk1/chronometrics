package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"eventmetrics/internal/domain"
)

func (h *Handler) handleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "content-type must be application/json")
		return
	}
	ctx := r.Context()
	defer func() { _ = r.Body.Close() }()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body too large")
			return
		}
		writeError(w, http.StatusBadRequest, "READ_ERROR", "failed to read request body")
		return
	}
	var ev domain.RawEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON")
		return
	}
	if err := h.ingester.Ingest(ctx, &ev); err != nil {
		if domain.IsValidationError(err) {
			if h.metrics != nil {
				h.metrics.IngestErrorsTotal.WithLabelValues("validation").Inc()
			}
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
		if errors.Is(err, domain.ErrPublishFailed) {
			if h.metrics != nil {
				h.metrics.IngestErrorsTotal.WithLabelValues("publish").Inc()
			}
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusServiceUnavailable, "PUBLISH_FAILED", "service overloaded, retry later")
			return
		}
		if h.metrics != nil {
			h.metrics.IngestErrorsTotal.WithLabelValues("internal").Inc()
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	if h.metrics != nil {
		h.metrics.EventsIngestedTotal.WithLabelValues("single").Inc()
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}
