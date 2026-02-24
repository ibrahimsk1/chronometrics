package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	"eventmetrics/internal/domain"
)

// fakePublisher implements EventPublisher for unit tests.
type fakePublisher struct {
	published []domain.Event
}

func (f *fakePublisher) Publish(ctx context.Context, e domain.Event) error {
	f.published = append(f.published, e)
	return nil
}

func (f *fakePublisher) Close(ctx context.Context) error { return nil }

func TestIngest_ValidEvent(t *testing.T) {
	fp := &fakePublisher{}
	uc := NewUseCase(fp, time.Hour, 24*time.Hour)

	raw := &domain.RawEvent{
		ID:        "evt1",
		Type:      "product_view",
		Timestamp: time.Now().Unix(), // seconds -> normalized to ms
		Data: map[string]interface{}{
			"channel":     "web",
			"campaign_id": "cmp",
			"tags":        []interface{}{"a", "b"},
		},
	}

	if err := uc.Ingest(context.Background(), raw); err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}
	if len(fp.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(fp.published))
	}
	p := fp.published[0]
	if p.ID != raw.ID || p.Type != raw.Type {
		t.Fatalf("published event mismatch: got %+v", p)
	}
	if p.TimestampMS == 0 {
		t.Fatalf("expected normalized timestamp_ms to be set")
	}
	if p.PayloadHash == "" {
		t.Fatalf("expected payload hash to be set")
	}
}

func TestIngest_MissingFields(t *testing.T) {
	fp := &fakePublisher{}
	uc := NewUseCase(fp, time.Hour, 24*time.Hour)
	raw := &domain.RawEvent{} // missing required fields
	if err := uc.Ingest(context.Background(), raw); err == nil {
		t.Fatalf("expected validation error for missing fields")
	}
	if len(fp.published) != 0 {
		t.Fatalf("expected no published events on validation failure")
	}
}

func TestIngest_InvalidTimestamp(t *testing.T) {
	fp := &fakePublisher{}
	uc := NewUseCase(fp, time.Hour, 24*time.Hour)
	raw := &domain.RawEvent{
		ID:        "e",
		Type:      "t",
		Timestamp: 1e17, // too large -> validation fail
	}
	if err := uc.Ingest(context.Background(), raw); err == nil {
		t.Fatalf("expected validation error for out-of-bounds timestamp")
	}
}

type failingPublisher struct{}

func (f *failingPublisher) Publish(ctx context.Context, e domain.Event) error {
	return errors.New("publish error")
}
func (f *failingPublisher) Close(ctx context.Context) error { return nil }

func TestIngest_PublishFailed(t *testing.T) {
	fp := &failingPublisher{}
	uc := NewUseCase(fp, time.Hour, 24*time.Hour)
	raw := &domain.RawEvent{
		ID:        "evt",
		Type:      "t",
		Timestamp: time.Now().Unix(),
	}
	err := uc.Ingest(context.Background(), raw)
	if err == nil {
		t.Fatalf("expected publish failure error")
	}
	if !errors.Is(err, domain.ErrPublishFailed) {
		t.Fatalf("expected error to be domain.ErrPublishFailed, got: %v", err)
	}
}
