package observability

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds all Prometheus metrics for chronometrics.
type Metrics struct {
	Registry *prometheus.Registry

	// HTTP layer
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec

	// Ingest
	EventsIngestedTotal *prometheus.CounterVec
	IngestErrorsTotal   *prometheus.CounterVec

	// Buffer
	BufferFlushesTotal *prometheus.CounterVec
	BufferDepth        prometheus.Gauge

	// Repository (ClickHouse)
	DBInsertDuration *prometheus.HistogramVec
	DBQueryDuration  *prometheus.HistogramVec
}

// New creates a Metrics instance backed by a fresh Prometheus registry.
func New() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		Registry: reg,

		HTTPRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "chronometrics_http_requests_total",
			Help: "Total number of HTTP requests by method, path, and status code.",
		}, []string{"method", "path", "status"}),

		HTTPRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "chronometrics_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path"}),

		EventsIngestedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "chronometrics_events_ingested_total",
			Help: "Total number of events accepted for ingestion.",
		}, []string{"type"}),

		IngestErrorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "chronometrics_ingest_errors_total",
			Help: "Total number of ingestion errors by reason.",
		}, []string{"reason"}),

		BufferFlushesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "chronometrics_buffer_flushes_total",
			Help: "Total number of buffer flush attempts by result.",
		}, []string{"result"}),

		BufferDepth: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "chronometrics_buffer_depth",
			Help: "Current number of events in the in-memory buffer.",
		}),

		DBInsertDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "chronometrics_db_insert_duration_seconds",
			Help:    "ClickHouse batch insert latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"result"}),

		DBQueryDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "chronometrics_db_query_duration_seconds",
			Help:    "ClickHouse query latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"result"}),
	}

	reg.MustRegister(
		m.HTTPRequestsTotal,
		m.HTTPRequestDuration,
		m.EventsIngestedTotal,
		m.IngestErrorsTotal,
		m.BufferFlushesTotal,
		m.BufferDepth,
		m.DBInsertDuration,
		m.DBQueryDuration,
	)

	return m
}
