package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

var ErrPublishFailed = errors.New("publish failed")

func (h *Handler) handleIngest(w http.ResponseWriter, r *http.Request) {
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
	var ev RawEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON")
		return
	}
	// basic validation
	if ev.EventName == "" || ev.UserID == "" || ev.Timestamp == 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "missing required fields")
		return
	}
	// call usecase
	if err := h.ingester.Ingest(ctx, &ev); err != nil {
		if errors.Is(err, ErrPublishFailed) {
			// advise retry-after (best-effort)
			w.Header().Set("Retry-After", "5")
			writeError(w, http.StatusServiceUnavailable, "PUBLISH_FAILED", "publish failed")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	// accepted
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) handleIngestBulk(w http.ResponseWriter, r *http.Request) {
	// bulk handler not implemented yet
	writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "bulk ingestion not implemented")
}
