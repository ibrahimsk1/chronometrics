package buffer

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"eventmetrics/internal/config"
	"eventmetrics/internal/domain"
	"eventmetrics/internal/observability"
)

// Flusher writes a batch of events to persistent storage.
type Flusher interface {
	Flush(ctx context.Context, batch []domain.Event) error
}

// Buffer is an in-memory bounded buffer for domain.Event.
type Buffer struct {
	ch           chan domain.Event
	capacity     int
	cfg          config.BufferConfig
	done         chan struct{}
	wg           sync.WaitGroup
	flusher      Flusher
	flushTrigger chan []domain.Event
	metrics      *observability.Metrics
	logger       *slog.Logger
}

// New creates a Buffer with the provided configuration.
// Call Start to begin background flushing.
func New(ctx context.Context, cfg config.BufferConfig, metrics *observability.Metrics, log *slog.Logger) *Buffer {
	capacity := cfg.Capacity
	if capacity <= 0 {
		capacity = 1000
	}
	if cfg.FlushBatchSize <= 0 {
		cfg.FlushBatchSize = 1000
	}
	return &Buffer{
		ch:           make(chan domain.Event, capacity),
		capacity:     capacity,
		cfg:          cfg,
		done:         make(chan struct{}),
		flusher:      nil,
		flushTrigger: make(chan []domain.Event, 4),
		metrics:      metrics,
		logger:       log.With("component", "buffer"),
	}
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

func (b *Buffer) dispatch(batch []domain.Event) {
	if len(batch) == 0 {
		return
	}
	out := make([]domain.Event, len(batch))
	copy(out, batch)
	b.flushTrigger <- out
}

func (b *Buffer) runFlush(ctx context.Context) {
	defer b.wg.Done()
	defer close(b.flushTrigger)

	for i := 0; i < 4; i++ {
		b.wg.Add(1)
		go b.flushOnce(ctx)
	}

	interval := b.cfg.FlushIntervalDuration
	if interval == 0 {
		interval = time.Duration(b.cfg.FlushInterval) * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	batch := make([]domain.Event, 0, b.cfg.FlushBatchSize)

	for {
		select {
		case e := <-b.ch:
			batch = append(batch, e)
			if len(batch) >= b.cfg.FlushBatchSize {
				b.dispatch(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			b.dispatch(batch)
			batch = batch[:0]
		case <-b.done:
			b.dispatch(batch)
			for {
				batch := b.drainBatch()
				if len(batch) == 0 {
					return
				}
				b.dispatch(batch)
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
	defer b.wg.Done()

	if b.flusher == nil {
		return
	}
	for batch := range b.flushTrigger {
		_ = b.flushWithRetry(ctx, batch)
	}
}

func (b *Buffer) flushWithRetry(parentCtx context.Context, batch []domain.Event) error {
	var lastErr error
	backoff := 10 * time.Millisecond
	for i := 0; i < max(1, b.cfg.FlushRetries); i++ {
		timeout := b.cfg.FlushTimeout
		if timeout == 0 {
			timeout = time.Duration(b.cfg.FlushTimeoutMs) * time.Millisecond
		}
		ctx, cancel := context.WithTimeout(parentCtx, timeout)
		err := b.flusher.Flush(ctx, batch)
		cancel()
		if err == nil {
			if b.metrics != nil {
				b.metrics.BufferFlushesTotal.WithLabelValues("success").Inc()
				b.metrics.BufferDepth.Set(float64(len(b.ch)))
			}
			return nil
		}
		lastErr = err
		time.Sleep(backoff)
		backoff *= 2
		if backoff > time.Second {
			backoff = time.Second
		}
	}
	if b.metrics != nil {
		b.metrics.BufferFlushesTotal.WithLabelValues("error").Inc()
	}
	b.logger.Error("flush failed after retries", "batch_size", len(batch), "error", lastErr)
	return lastErr
}

// Publish attempts to admit an event into the bounded buffer without blocking.
// Returns domain.ErrPublishFailed when the context is cancelled or the buffer is full.
// Fires an early flush trigger when the batch threshold is reached.
func (b *Buffer) Publish(ctx context.Context, ev domain.Event) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("context cancelled: %w", domain.ErrPublishFailed)
	default:
	}
	select {
	case b.ch <- ev:
		if b.metrics != nil {
			b.metrics.BufferDepth.Set(float64(len(b.ch)))
		}
		return nil
	default:
		return fmt.Errorf("buffer at capacity (%d): %w", b.capacity, domain.ErrPublishFailed)
	}
}

// Close signals shutdown, drains remaining events, and waits for the flush goroutine.
// The provided context bounds the wait: if it expires before the drain completes,
// Close returns ctx.Err() but the drain goroutine continues to completion in the background.
func (b *Buffer) Close(ctx context.Context) error {
	select {
	case <-b.done:
		return nil
	default:
		close(b.done)
		finished := make(chan struct{})
		go func() {
			b.wg.Wait()
			close(finished)
		}()
		select {
		case <-finished:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
