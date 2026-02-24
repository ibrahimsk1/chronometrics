package app

import (
	"context"
	"log"

	"eventmetrics/internal/config"
)

// Base holds shared application resources.
type Base struct {
	Config config.Config
}

// Setup loads configuration and returns a Base with the validated config.
func Setup(ctx context.Context) (*Base, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	// Minimal logging to surface loaded configuration (non-sensitive fields).
	log.Printf("config loaded: strategy=%s server.port=%d buffer.capacity=%d", cfg.Strategy, cfg.Server.Port, cfg.Buffer.Capacity)
	return &Base{Config: cfg}, nil
}

