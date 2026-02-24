package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// BulkRequest is the expected request shape for bulk ingestion.
type BulkRequest struct {
	Events []RawEvent `json:"events"`
}

// BulkResponse reports accepted/rejected counts and optional errors.
type BulkResponse struct {
	Accepted int      `json:"accepted"`
	Rejected int      `json:"rejected"`
	Errors   []string `json:"errors,omitempty"`
}

func (h *Handler) handleBulk(w http.ResponseWriter, r *http.Request) {
	// enforce body limit
	max := h.cfg.MaxBodyBytes
	if max <= 0 {
		max = 1 << 20 // 1MB
	}
	r.Body = http.MaxBytesReader(w, r.Body, max)
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body too large")
		return
	}
	var req BulkRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON")
		return
	}
	if len(req.Events) == 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "no events")
		return
	}

	var resp BulkResponse
	for i := range req.Events {
		ev := &req.Events[i]
		if ev.EventName == "" || ev.UserID == "" || ev.Timestamp == 0 {
			resp.Rejected++
			resp.Errors = append(resp.Errors, "missing required fields for item")
			continue
		}
		if err := h.ingester.Ingest(r.Context(), ev); err != nil {
			if errors.Is(err, ErrPublishFailed) {
				resp.Rejected++
				resp.Errors = append(resp.Errors, "publish failed for item")
				continue
			}
			resp.Rejected++
			resp.Errors = append(resp.Errors, "internal error for item")
			continue
		}
		resp.Accepted++
	}

	// If nothing accepted, treat as client error if only validation issues, else 503 if all publish failed.
	if resp.Accepted == 0 {
		// if any error message contains "publish failed" => service unavailable
		for _, e := range resp.Errors {
			if e == "publish failed for item" {
				w.Header().Set("Retry-After", "5")
				writeError(w, http.StatusServiceUnavailable, "PUBLISH_FAILED", "all publishes failed")
				return
			}
		}
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "no valid events")
		return
	}

	// Partial or full success: return accepted/rejected counts
	writeJSON(w, http.StatusAccepted, resp)
}
