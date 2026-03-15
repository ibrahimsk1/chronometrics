package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"eventmetrics/internal/domain"
	"eventmetrics/internal/health"
	"eventmetrics/internal/observability"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Handler is the HTTP adapter entrypoint.
type Handler struct {
	ingester Ingester
	querier  MetricsQuerier
	health   HealthChecker
	cfg      ServerConfig
	mux      *http.ServeMux
	limiters LimiterRegistry
	logger   *slog.Logger
	metrics  *observability.Metrics
}

type RateLimiter struct {
	count int
	limit int
	mu    sync.Mutex
}

// LimiterRegistry maps route names to their individual rate limiters.
type LimiterRegistry map[string]*RateLimiter

// RateLimiterConfig holds per-route request limits (requests/second).
type RateLimiterConfig struct {
	Ingest  int // e.g. 25000/s
	Bulk    int // e.g. 500/s
	Metrics int // e.g. 100/s
	Health  int // e.g. 60/s
}

// New constructs a Handler with required dependencies.
func New(ing Ingester, q MetricsQuerier, h HealthChecker, cfg ServerConfig, limiters LimiterRegistry, log *slog.Logger, metrics *observability.Metrics) *Handler {
	hnd := &Handler{
		ingester: ing,
		querier:  q,
		health:   h,
		cfg:      cfg,
		mux:      http.NewServeMux(),
		limiters: limiters,
		logger:   log.With("component", "handler"),
		metrics:  metrics,
	}
	// register routes — each route gets its own named limiter
	hnd.mux.Handle("/events", hnd.withLimiter("ingest", hnd.handleIngest))
	hnd.mux.Handle("/events/bulk", hnd.withLimiter("bulk", hnd.handleBulk))
	hnd.mux.Handle("/metrics", hnd.withLimiter("metrics", hnd.handleMetrics))
	hnd.mux.Handle("/health", hnd.withLimiter("health", hnd.handleHealth))
	if metrics != nil {
		hnd.mux.Handle("/prometheus", promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{}))
	}
	return hnd
}

// NewLimiterRegistry creates a LimiterRegistry from a config, starting a single
// reset goroutine that clears all counters every second.
func NewLimiterRegistry(ctx context.Context, cfg RateLimiterConfig) LimiterRegistry {
	registry := LimiterRegistry{
		"ingest":  {limit: cfg.Ingest},
		"bulk":    {limit: cfg.Bulk},
		"metrics": {limit: cfg.Metrics},
		"health":  {limit: cfg.Health},
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				for _, rl := range registry {
					rl.mu.Lock()
					rl.count = 0
					rl.mu.Unlock()
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return registry
}

// Router returns the http.Handler for the service.
func (h *Handler) Router() http.Handler {
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
	if h.health != nil {
		writeJSON(w, http.StatusOK, h.health.Health(r.Context()))
		return
	}
	writeJSON(w, http.StatusOK, health.HealthStatus{Status: "healthy"})
}
