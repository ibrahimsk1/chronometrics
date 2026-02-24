package handler

import (
	"errors"
	"net/http"
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
	// Parse query params and delegate to MetricsQuerier.
	params := r.URL.Query()
	if h.querier == nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "no querier configured")
		return
	}
	res, err := h.querier.Query(r.Context(), params)
	if err != nil {
		// If backend timed out, map to 504
		if errors.Is(err, ErrQueryTimeout) {
			writeError(w, http.StatusGatewayTimeout, "QUERY_TIMEOUT", "query timed out")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Minimal health handler stub: delegates to HealthChecker if present.
	if h.health != nil {
		if s, err := h.health.Health(r.Context()); err == nil {
			writeJSON(w, http.StatusOK, s)
			return
		}
		// If health check failed, still return 200 but include error field for now.
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "degraded"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}
