package buffer

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	ch           chan domain.Event
	capacity     int
	cfg          config.BufferConfig
	done         chan struct{}
	wg           sync.WaitGroup
	flusher      Flusher
	flushTrigger chan struct{}
}

// New creates a Buffer with the provided Flusher and configuration.
// It does not start background goroutines yet.
func New(ctx context.Context, _ Flusher, cfg config.BufferConfig) *Buffer {
	capacity := cfg.Capacity
	if capacity <= 0 {
		capacity = 1000
	}
	b := &Buffer{
		ch:           make(chan domain.Event, capacity),
		capacity:     capacity,
		cfg:          cfg,
		done:         make(chan struct{}),
		flusher:      nil,
		flushTrigger: make(chan struct{}, 1),
	}
	// no goroutine started here; will be started when a flusher is provided in a later CU
	return b
}

// Start begins the flush goroutine using the provided Flusher.
func (b *Buffer) Start(ctx context.Context, fl Flusher) {
	if fl == nil {
		return
	}
	b.flusher = fl
	b.wg.Add(1)
	go b.runFlush(ctx)
}

func (b *Buffer) runFlush(ctx context.Context) {
	defer b.wg.Done()
	interval := b.cfg.FlushIntervalDuration
	if interval == 0 {
		interval = time.Duration(b.cfg.FlushInterval) * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			b.flushOnce(ctx)
		case <-b.flushTrigger:
			b.flushOnce(ctx)
		case <-b.done:
			// final drain
			for {
				batch := b.drainBatch()
				if len(batch) == 0 {
					return
				}
				_ = b.flushWithRetry(ctx, batch)
			}
		}
	}
}

func (b *Buffer) drainBatch() []domain.Event {
	batch := make([]domain.Event, 0, b.cfg.FlushBatchSize)
	for i := 0; i < b.cfg.FlushBatchSize; i++ {
		select {
		case ev := <-b.ch:
			batch = append(batch, ev)
		default:
			return batch
		}
	}
	return batch
}

func (b *Buffer) flushOnce(ctx context.Context) {
	if b.flusher == nil {
		return
	}
	batch := b.drainBatch()
	if len(batch) == 0 {
		return
	}
	_ = b.flushWithRetry(ctx, batch)
}

func (b *Buffer) flushWithRetry(parentCtx context.Context, batch []domain.Event) error {
	var lastErr error
	for i := 0; i < max(1, b.cfg.FlushRetries); i++ {
		timeout := b.cfg.FlushTimeout
		if timeout == 0 {
			timeout = time.Duration(b.cfg.FlushTimeoutMs) * time.Millisecond
		}
		ctx, cancel := context.WithTimeout(parentCtx, timeout)
		err := b.flusher.Flush(ctx, batch)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		// small backoff
		time.Sleep(10 * time.Millisecond)
	}
	return lastErr
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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
	// If no flusher/goroutine was started, nothing to do.
	select {
	case <-b.done:
		return nil
	default:
		// signal done and wait for flush goroutine if it exists
		close(b.done)
		b.wg.Wait()
		return nil
	}
}
