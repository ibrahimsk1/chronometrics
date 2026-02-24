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
	hnd.mux.Handle("/events/bulk", http.HandlerFunc(hnd.handleIngestBulk))
	hnd.mux.Handle("/metrics", http.HandlerFunc(hnd.handleMetrics))
	hnd.mux.Handle("/health", http.HandlerFunc(hnd.handleHealth))
	return hnd
}

// Router returns the http.Handler for the service.
func (h *Handler) Router() http.Handler {
	return h.mux
}
