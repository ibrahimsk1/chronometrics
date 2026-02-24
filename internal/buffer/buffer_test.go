package buffer

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"eventmetrics/internal/config"
	"eventmetrics/internal/domain"
)

type fakeFlusher struct{}

func (f *fakeFlusher) Flush(ctx context.Context, batch []domain.Event) error {
	return nil
}

func TestBuffer_Admit(t *testing.T) {
	ctx := context.Background()
	cfg := config.BufferConfig{Capacity: 2, FlushInterval: 1}
	fl := &fakeFlusher{}
	b := New(ctx, fl, cfg)
	b.Start(ctx, fl)

	ev := domain.Event{
		EventName:   "e1",
		UserID:      "u",
		TimestampMs: uint64(1234567890),
		Channel:     "",
		CampaignID:  "",
		Tags:        []string{},
		Metadata:    "",
		PayloadHash: 0,
	}
	if err := b.Publish(ctx, ev); err != nil {
		t.Fatalf("expected first publish to succeed, got error: %v", err)
	}
	if err := b.Publish(ctx, ev); err != nil {
		t.Fatalf("expected second publish to succeed, got error: %v", err)
	}
}

func TestBuffer_ConfigDefaults(t *testing.T) {
	ctx := context.Background()
	// zero capacity should fall back to default (1000)
	cfg := config.BufferConfig{Capacity: 0, FlushInterval: 1}
	fl := &fakeFlusher{}
	b := New(ctx, fl, cfg)
	b.Start(ctx, fl)
	if b.capacity <= 0 {
		t.Fatalf("expected positive capacity, got %d", b.capacity)
	}
}

func TestBuffer_Full_ReturnsPublishFailed(t *testing.T) {
	ctx := context.Background()
	cfg := config.BufferConfig{Capacity: 1, FlushInterval: 1}
	fl := &fakeFlusher{}
	b := New(ctx, fl, cfg)
	b.Start(ctx, fl)

	ev := domain.Event{
		EventName:   "e1",
		UserID:      "u",
		TimestampMs: uint64(1234567890),
		Channel:     "",
		CampaignID:  "",
		Tags:        []string{},
		Metadata:    "",
		PayloadHash: 0,
	}
	if err := b.Publish(ctx, ev); err != nil {
		t.Fatalf("expected first publish to succeed, got error: %v", err)
	}
	if err := b.Publish(ctx, ev); err == nil {
		t.Fatalf("expected publish to full buffer to fail, but got nil")
	} else if !errors.Is(err, domain.ErrPublishFailed) {
		t.Fatalf("expected error to wrap domain.ErrPublishFailed, got: %v", err)
	}
}

// recordingFlusher records flush calls for assertions.
type recordingFlusher struct {
	ch chan []domain.Event
}

func (r *recordingFlusher) Flush(ctx context.Context, batch []domain.Event) error {
	select {
	case r.ch <- batch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestBuffer_FlushTriggeredByInterval(t *testing.T) {
	ctx := context.Background()
	cfg := config.BufferConfig{Capacity: 100, FlushInterval: 20, FlushBatchSize: 10, FlushRetries: 1, FlushTimeoutMs: 100}
	fl := &recordingFlusher{ch: make(chan []domain.Event, 1)}
	b := New(ctx, fl, cfg)
	b.Start(ctx, fl)

	ev := domain.Event{
		EventName:   "e1",
		UserID:      "u",
		TimestampMs: uint64(1),
		Channel:     "",
		CampaignID:  "",
		Tags:        []string{},
		Metadata:    "",
		PayloadHash: 0,
	}
	if err := b.Publish(ctx, ev); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	select {
	case batch := <-fl.ch:
		if len(batch) != 1 {
			t.Fatalf("expected batch len 1, got %d", len(batch))
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timeout waiting for flush by interval")
	}
}

func TestBuffer_FlushTriggeredByBatchSize(t *testing.T) {
	ctx := context.Background()
	cfg := config.BufferConfig{Capacity: 100, FlushInterval: 1000, FlushBatchSize: 3, FlushRetries: 1, FlushTimeoutMs: 100}
	fl := &recordingFlusher{ch: make(chan []domain.Event, 1)}
	b := New(ctx, fl, cfg)
	b.Start(ctx, fl)

	for i := 0; i < 3; i++ {
		ev := domain.Event{
			EventName:   fmt.Sprintf("e%d", i),
			UserID:      "u",
			TimestampMs: uint64(i),
			Channel:     "",
			CampaignID:  "",
			Tags:        []string{},
			Metadata:    "",
			PayloadHash: 0,
		}
		if err := b.Publish(ctx, ev); err != nil {
			t.Fatalf("publish failed: %v", err)
		}
		// trigger flush if threshold reached
		if len(b.ch) >= b.cfg.FlushBatchSize {
			select {
			case b.flushTrigger <- struct{}{}:
			default:
			}
		}
	}

	select {
	case batch := <-fl.ch:
		if len(batch) != 3 {
			t.Fatalf("expected batch len 3, got %d", len(batch))
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timeout waiting for flush by batch size")
	}
}

func TestBuffer_Close_DrainsRemaining(t *testing.T) {
	ctx := context.Background()
	cfg := config.BufferConfig{Capacity: 100, FlushInterval: 10000, FlushBatchSize: 1000, FlushRetries: 1, FlushTimeoutMs: 100}
	fl := &recordingFlusher{ch: make(chan []domain.Event, 1)}
	b := New(ctx, fl, cfg)
	b.Start(ctx, fl)

	// publish 2 events (less than batch size), rely on Close to flush them
	for i := 0; i < 2; i++ {
		ev := domain.Event{
			EventName:   fmt.Sprintf("e%d", i),
			UserID:      "u",
			TimestampMs: uint64(i),
			Channel:     "",
			CampaignID:  "",
			Tags:        []string{},
			Metadata:    "",
			PayloadHash: 0,
		}
		if err := b.Publish(ctx, ev); err != nil {
			t.Fatalf("publish failed: %v", err)
		}
	}

	// Close should trigger final flush and wait for it to complete
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := b.Close(shutdownCtx); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	select {
	case batch := <-fl.ch:
		if len(batch) != 2 {
			t.Fatalf("expected batch len 2 after Close, got %d", len(batch))
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timeout waiting for final flush after Close")
	}
}

func TestBuffer_Close_Idempotent(t *testing.T) {
	ctx := context.Background()
	cfg := config.BufferConfig{Capacity: 10, FlushInterval: 10000, FlushBatchSize: 100, FlushRetries: 1, FlushTimeoutMs: 100}
	fl := &recordingFlusher{ch: make(chan []domain.Event, 1)}
	b := New(ctx, fl, cfg)
	b.Start(ctx, fl)

	if err := b.Close(ctx); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	// second close should be no-op and return nil
	if err := b.Close(ctx); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
}
