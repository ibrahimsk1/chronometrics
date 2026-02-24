package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"eventmetrics/internal/app"
	"eventmetrics/internal/handler"
	"eventmetrics/internal/ingest"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	base, err := app.Setup(ctx)
	if err != nil {
		log.Fatalf("setup failed: %v", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		base.Shutdown(shutdownCtx)
	}()

	// Create use-case with validation windows from config (seconds -> time.Duration).
	uc := ingest.NewUseCase(base.Publisher,
		base.Config.Validation.MaxFutureDuration,
		base.Config.Validation.MaxPastDuration,
	)

	ing := handler.NewUseCaseAdapter(uc)

	// Use repository for metrics queries and health if available.
	var healthChecker handler.HealthChecker = &simpleHealth{
		status: handler.HealthStatus{
			Status:         "healthy",
			BufferStrategy: base.Config.Strategy,
		},
	}
	var querier handler.MetricsQuerier
	if base.Repository != nil {
		healthChecker = base.Repository
		querier = base.Repository
	}

	h := handler.New(ing, querier, healthChecker, handler.ServerConfig{MaxBodyBytes: 1 << 20})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", base.Config.Server.Port),
		Handler: h.Router(),
	}

	// Start server
	go func() {
		log.Printf("starting ingestor on %s (strategy=%s)", srv.Addr, base.Config.Strategy)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	log.Printf("shutting down ingestor")
	_ = srv.Shutdown(shutdownCtx)
	base.Shutdown(shutdownCtx)
	log.Printf("ingestor shutdown complete")
}

// simpleHealth adapts a HealthStatus for the handler.HealthChecker interface.
type simpleHealth struct {
	status handler.HealthStatus
}

func (s *simpleHealth) Health(ctx context.Context) handler.HealthStatus {
	return s.status
}
