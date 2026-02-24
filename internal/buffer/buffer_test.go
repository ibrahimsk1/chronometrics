package buffer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"eventmetrics/internal/config"
	"eventmetrics/internal/domain"
)

func BenchmarkBuffer_Publish(b *testing.B) {
	ctx := context.Background()
	cfg := config.BufferConfig{
		Capacity:              100_000,
		FlushIntervalDuration: 50 * time.Millisecond,
		FlushInterval:         50,
		FlushBatchSize:        5000,
		FlushRetries:          1,
		FlushTimeout:          1 * time.Second,
		FlushTimeoutMs:        1000,
	}
	fl := &fakeFlusher{}
	buf := New(ctx, cfg)
	buf.Start(ctx, fl)
	defer buf.Close(ctx)

	ev := domain.Event{EventName: "bench", UserID: "u", TimestampMs: 1}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = buf.Publish(ctx, ev)
		}
	})
}

type fakeFlusher struct{}

func (f *fakeFlusher) Flush(ctx context.Context, batch []domain.Event) error {
	return nil
}

func TestBuffer_Admit(t *testing.T) {
	ctx := context.Background()
	cfg := config.BufferConfig{Capacity: 2, FlushInterval: 1}
	fl := &fakeFlusher{}
	b := New(ctx, cfg)
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
	b := New(ctx, cfg)
	b.Start(ctx, fl)
	if b.capacity <= 0 {
		t.Fatalf("expected positive capacity, got %d", b.capacity)
	}
}

func TestBuffer_Full_ReturnsPublishFailed(t *testing.T) {
	ctx := context.Background()
	cfg := config.BufferConfig{Capacity: 1, FlushInterval: 1}
	fl := &fakeFlusher{}
	b := New(ctx, cfg)
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
	cfg := config.BufferConfig{
		Capacity:              100,
		FlushInterval:         20,
		FlushIntervalDuration: 20 * time.Millisecond,
		FlushBatchSize:        10,
		FlushRetries:          1,
		FlushTimeoutMs:        100,
		FlushTimeout:          100 * time.Millisecond,
	}
	fl := &recordingFlusher{ch: make(chan []domain.Event, 1)}
	b := New(ctx, cfg)
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
	b := New(ctx, cfg)
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
	b := New(ctx, cfg)
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
	b := New(ctx, cfg)
	b.Start(ctx, fl)

	if err := b.Close(ctx); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	// second close should be no-op and return nil
	if err := b.Close(ctx); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
}

// TestBuffer_PeakLoad_20k simulates 20 goroutines each publishing 1,000 events
// (~20k total) using the recommended production config. Drop rate must stay under 1%.
func TestBuffer_PeakLoad_20k(t *testing.T) {
	ctx := context.Background()
	cfg := config.BufferConfig{
		Capacity:              50_000,
		FlushIntervalDuration: 50 * time.Millisecond,
		FlushInterval:         50,
		FlushBatchSize:        5000,
		FlushRetries:          1,
		FlushTimeout:          1 * time.Second,
		FlushTimeoutMs:        1000,
	}
	fl := &fakeFlusher{}
	buf := New(ctx, cfg)
	buf.Start(ctx, fl)
	defer buf.Close(ctx)

	const (
		workers   = 20
		perWorker = 1_000 // 20 × 1000 = 20,000 total
	)
	var (
		wg      sync.WaitGroup
		dropped int64
	)
	start := time.Now()
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				ev := domain.Event{
					EventName:   "load",
					UserID:      fmt.Sprintf("u%d", id),
					TimestampMs: uint64(j),
				}
				if err := buf.Publish(ctx, ev); err != nil {
					atomic.AddInt64(&dropped, 1)
				}
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	total := int64(workers * perWorker)
	dropRate := float64(dropped) / float64(total) * 100
	t.Logf("elapsed=%v total=%d dropped=%d drop_rate=%.2f%%", elapsed, total, dropped, dropRate)

	if dropRate > 1.0 {
		t.Errorf("drop rate %.2f%% exceeds 1%% threshold with tuned config", dropRate)
	}
}

// TestBuffer_DefaultConfig_DropsAtPeak documents that the default config (capacity=1000)
// cannot absorb a 20k burst — events are dropped and ErrPublishFailed is returned.
func TestBuffer_DefaultConfig_DropsAtPeak(t *testing.T) {
	ctx := context.Background()
	cfg := config.BufferConfig{
		Capacity:              1000,
		FlushIntervalDuration: 100 * time.Millisecond,
		FlushInterval:         100,
		FlushBatchSize:        1000,
		FlushRetries:          1,
		FlushTimeout:          100 * time.Millisecond,
		FlushTimeoutMs:        100,
	}
	fl := &fakeFlusher{}
	buf := New(ctx, cfg)
	buf.Start(ctx, fl)
	defer buf.Close(ctx)

	var dropped int64
	ev := domain.Event{EventName: "e", UserID: "u", TimestampMs: 1}
	// Fire 20,000 publishes as fast as possible (simulates a burst)
	for i := 0; i < 20_000; i++ {
		if err := buf.Publish(ctx, ev); err != nil {
			atomic.AddInt64(&dropped, 1)
		}
	}
	t.Logf("dropped %d / 20000 with default config (capacity=1000)", dropped)
	if dropped == 0 {
		t.Error("expected drops with default capacity=1000 under 20k burst; none observed")
	}
}
