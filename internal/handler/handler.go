package handler

import (
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
	return h.mux
}

func (h *Handler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// Minimal metrics handler stub: responds 200 with empty result.
	writeJSON(w, http.StatusOK, map[string]interface{}{"metrics": []interface{}{}})
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
