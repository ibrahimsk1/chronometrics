package buffer

import (
	"context"
	"fmt"

	"eventmetrics/internal/config"
	"eventmetrics/internal/domain"
)

// Flusher writes a batch of events to persistent storage.
type Flusher interface {
	Flush(ctx context.Context, batch []domain.Event) error
}

// Buffer is an in-memory bounded buffer for domain.Event.
// This file provides the types and constructor; flushing goroutine is added in later CUs.
type Buffer struct {
	ch       chan domain.Event
	capacity int
	cfg      config.BufferConfig
	done     chan struct{}
}

// New creates a Buffer with the provided Flusher and configuration.
// It does not start background goroutines yet.
func New(ctx context.Context, _ Flusher, cfg config.BufferConfig) *Buffer {
	capacity := cfg.Capacity
	if capacity <= 0 {
		capacity = 1000
	}
	return &Buffer{
		ch:       make(chan domain.Event, capacity),
		capacity: capacity,
		cfg:      cfg,
		done:     make(chan struct{}),
	}
}

// Publish attempts to admit an event into the bounded buffer without blocking.
// Returns domain.ErrPublishFailed when the buffer is full.
func (b *Buffer) Publish(ctx context.Context, ev domain.Event) error {
	select {
	case b.ch <- ev:
		return nil
	default:
		return fmt.Errorf("buffer at capacity (%d): %w", b.capacity, domain.ErrPublishFailed)
	}
}

// Close is a placeholder for shutdown behavior (drain + final flush).
func (b *Buffer) Close(ctx context.Context) error {
	// No-op for this CU; implemented in a later CU.
	return nil
}
