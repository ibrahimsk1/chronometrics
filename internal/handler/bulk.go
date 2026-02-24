package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"eventmetrics/internal/domain"
)

// BulkRequest is the expected request shape for bulk ingestion.
type BulkRequest struct {
	Events []domain.RawEvent `json:"events"`
}

// BulkResponse reports accepted/rejected counts and optional errors.
type BulkResponse struct {
	Accepted int      `json:"accepted"`
	Rejected int      `json:"rejected"`
	Errors   []BulkItemError `json:"errors,omitempty"`
}

// BulkItemError describes an error for a specific item in the bulk payload.
type BulkItemError struct {
	Index int    `json:"index"`
	Error string `json:"error"`
	Code  string `json:"code"`
}

func (h *Handler) handleBulk(w http.ResponseWriter, r *http.Request) {
	// Only allow POST
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	// enforce body limit
	max := h.cfg.MaxBodyBytes
	if max <= 0 {
		max = 1 << 20 // 1MB
	}
	r.Body = http.MaxBytesReader(w, r.Body, max)
	defer func() { _ = r.Body.Close() }()
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
		if err := h.ingester.Ingest(r.Context(), ev); err != nil {
			resp.Rejected++
			// Validation error -> include code and message
			if domain.IsValidationError(err) {
				resp.Errors = append(resp.Errors, BulkItemError{Index: i, Error: err.Error(), Code: "VALIDATION_ERROR"})
				continue
			}
			// Publish failure -> service unavailable for that item
			if errors.Is(err, domain.ErrPublishFailed) {
				resp.Errors = append(resp.Errors, BulkItemError{Index: i, Error: "publish failed for item", Code: "PUBLISH_FAILED"})
				continue
			}
			// Unknown/internal
			resp.Errors = append(resp.Errors, BulkItemError{Index: i, Error: "internal error for item", Code: "INTERNAL_ERROR"})
			continue
		}
		resp.Accepted++
	}

	// If nothing accepted, treat as client error if only validation issues, else 503 if all publish failed.
	if resp.Accepted == 0 {
		// if any error message contains "publish failed" => service unavailable
		publishFailed := false
		for _, e := range resp.Errors {
			if e.Code == "PUBLISH_FAILED" {
				publishFailed = true
				break
			}
		}
		if publishFailed {
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusServiceUnavailable, "PUBLISH_FAILED", "all publishes failed")
			return
		}
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "no valid events")
		return
	}

	// Partial or full success: return accepted/rejected counts
	writeJSON(w, http.StatusAccepted, resp)
}
