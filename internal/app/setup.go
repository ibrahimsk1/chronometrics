package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"eventmetrics/internal/buffer"
	"eventmetrics/internal/config"
	"eventmetrics/internal/ingest"
)

// Base holds shared application resources.
type Base struct {
	Config    config.Config
	Publisher ingest.EventPublisher
	Buffer    *buffer.Buffer
}

// Setup loads configuration and returns a Base with the validated config and
// a strategy-appropriate publisher. Currently supports "memory" strategy.
func Setup(ctx context.Context) (*Base, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	base := &Base{Config: cfg}

	switch cfg.Strategy {
	case "memory", "":
		// Create in-memory buffer publisher.
		buf := buffer.New(ctx, nil, cfg.Buffer)
		base.Buffer = buf
		base.Publisher = buf
		// Start flush goroutine with no-op flusher for now.
		// TODO: provide real flusher (repository) in later CU.
		buf.Start(ctx, nil)
		log.Printf("memory buffer publisher created (capacity=%d)", buf)
	default:
		return nil, fmt.Errorf("unsupported buffer strategy: %s", cfg.Strategy)
	}

	// Minimal logging to surface loaded configuration (non-sensitive fields).
	log.Printf("config loaded: strategy=%s server.port=%d buffer.capacity=%d", cfg.Strategy, cfg.Server.Port, cfg.Buffer.Capacity)

	// Basic sanity check: ensure publisher is present.
	if base.Publisher == nil {
		return nil, errors.New("no publisher configured")
	}

	// Allow some time for background components to initialize (best-effort).
	time.Sleep(10 * time.Millisecond)

	return base, nil
}
