package buffer

import (
	"context"
	"errors"
	"testing"

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

	ev := domain.Event{ID: "e1", Type: "event_type", TimestampMS: 1234567890}
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
	if b.capacity <= 0 {
		t.Fatalf("expected positive capacity, got %d", b.capacity)
	}
}

func TestBuffer_Full_ReturnsPublishFailed(t *testing.T) {
	ctx := context.Background()
	cfg := config.BufferConfig{Capacity: 1, FlushInterval: 1}
	fl := &fakeFlusher{}
	b := New(ctx, fl, cfg)

	ev := domain.Event{ID: "e1", Type: "event_type", TimestampMS: 1234567890}
	if err := b.Publish(ctx, ev); err != nil {
		t.Fatalf("expected first publish to succeed, got error: %v", err)
	}
	if err := b.Publish(ctx, ev); err == nil {
		t.Fatalf("expected publish to full buffer to fail, but got nil")
	} else if !errors.Is(err, domain.ErrPublishFailed) {
		t.Fatalf("expected error to wrap domain.ErrPublishFailed, got: %v", err)
	}
}
