package handler

import (
	"errors"
	"net/http"
	"strconv"

	"eventmetrics/internal/domain"
)

// Handler is the HTTP adapter entrypoint.
type Handler struct {
	ingester Ingester
	querier  MetricsQuerier
	health   HealthChecker
	cfg      ServerConfig
	mux      *http.ServeMux
}

// New constructs a Handler with required dependencies.
func New(ing Ingester, q MetricsQuerier, h HealthChecker, cfg ServerConfig) *Handler {
	hnd := &Handler{
		ingester: ing,
		querier:  q,
		health:   h,
		cfg:      cfg,
		mux:      http.NewServeMux(),
	}
	// register routes
	hnd.mux.Handle("/events", http.HandlerFunc(hnd.handleIngest))
	hnd.mux.Handle("/events/bulk", http.HandlerFunc(hnd.handleBulk))
	hnd.mux.Handle("/metrics", http.HandlerFunc(hnd.handleMetrics))
	hnd.mux.Handle("/health", http.HandlerFunc(hnd.handleHealth))
	return hnd
}

// Router returns the http.Handler for the service.
func (h *Handler) Router() http.Handler {
	// Wrap mux with middleware so all routes get the common behavior.
	return h.withMiddleware(h.mux)
}

func (h *Handler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// Only allow GET
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	// Parse query params and delegate to MetricsQuerier.
	if h.querier == nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "no querier configured")
		return
	}
	q := r.URL.Query()
	eventName := q.Get("event_name")
	fromStr := q.Get("from")
	toStr := q.Get("to")
	groupBy := q.Get("group_by")

	// Parse timestamps (seconds or milliseconds accepted).
	var fromVal, toVal uint64
	if fromStr != "" {
		if v, err := strconv.ParseInt(fromStr, 10, 64); err == nil {
			fromVal = domain.NormalizeTimestamp(v)
		}
	}
	if toStr != "" {
		if v, err := strconv.ParseInt(toStr, 10, 64); err == nil {
			toVal = domain.NormalizeTimestamp(v)
		}
	}
	params := domain.QueryParams{
		EventName: eventName,
		From:      fromVal,
		To:        toVal,
		GroupBy:   groupBy,
	}
	// Validate query parameters before executing the query.
	if err := domain.ValidateQueryParams(&params); err != nil {
		// ValidationError -> 400
		if domain.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
		// Fallback to internal error for unexpected validation failures.
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}

	res, err := h.querier.Query(r.Context(), params)
	if err != nil {
		// If backend timed out, map to 504
		if errors.Is(err, domain.ErrQueryTimeout) {
			writeError(w, http.StatusGatewayTimeout, "QUERY_TIMEOUT", "query timed out")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Only allow GET
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	// Minimal health handler stub: delegates to HealthChecker if present.
	if h.health != nil {
		s := h.health.Health(r.Context())
		writeJSON(w, http.StatusOK, s)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}
