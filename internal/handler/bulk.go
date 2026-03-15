package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"eventmetrics/internal/domain"
)

const defaultMaxBulkEvents = 1000

// BulkRequest is the expected request shape for bulk ingestion.
type BulkRequest struct {
	Events []domain.RawEvent `json:"events"`
}

// BulkResponse reports accepted/rejected counts and optional errors.
type BulkResponse struct {
	Accepted int             `json:"accepted"`
	Rejected int             `json:"rejected"`
	Errors   []BulkItemError `json:"errors,omitempty"`
}

// BulkItemError describes an error for a specific item in the bulk payload.
type BulkItemError struct {
	Index int    `json:"index"`
	Error string `json:"error"`
	Code  string `json:"code"`
}

func (h *Handler) handleBulk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "content-type must be application/json")
		return
	}
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
	var req BulkRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON")
		return
	}
	if len(req.Events) == 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "no events")
		return
	}
	maxBulk := h.cfg.MaxBulkEvents
	if maxBulk <= 0 {
		maxBulk = defaultMaxBulkEvents
	}
	if len(req.Events) > maxBulk {
		writeError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", fmt.Sprintf("too many events: max %d", maxBulk))
		return
	}

	var resp BulkResponse
	for i := range req.Events {
		ev := &req.Events[i]
		if err := h.ingester.Ingest(r.Context(), ev); err != nil {
			resp.Rejected++
			if domain.IsValidationError(err) {
				if h.metrics != nil {
					h.metrics.IngestErrorsTotal.WithLabelValues("validation").Inc()
				}
				resp.Errors = append(resp.Errors, BulkItemError{Index: i, Error: err.Error(), Code: "VALIDATION_ERROR"})
				continue
			}
			if errors.Is(err, domain.ErrPublishFailed) {
				if h.metrics != nil {
					h.metrics.IngestErrorsTotal.WithLabelValues("publish").Inc()
				}
				resp.Errors = append(resp.Errors, BulkItemError{Index: i, Error: "publish failed for item", Code: "PUBLISH_FAILED"})
				continue
			}
			if h.metrics != nil {
				h.metrics.IngestErrorsTotal.WithLabelValues("internal").Inc()
			}
			resp.Errors = append(resp.Errors, BulkItemError{Index: i, Error: "internal error for item", Code: "INTERNAL_ERROR"})
			continue
		}
		resp.Accepted++
		if h.metrics != nil {
			h.metrics.EventsIngestedTotal.WithLabelValues("bulk").Inc()
		}
	}

	if resp.Accepted == 0 {
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

	writeJSON(w, http.StatusAccepted, resp)
}
