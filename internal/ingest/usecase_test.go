package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	"eventmetrics/internal/domain"
)

// nopPublisher implements EventPublisher for constructor tests.
type nopPublisher struct{}

func (n *nopPublisher) Publish(ctx context.Context, e domain.Event) error { return nil }
func (n *nopPublisher) Close(ctx context.Context) error                   { return nil }

// fakePublisher implements EventPublisher for unit tests.
type fakePublisher struct {
	published []domain.Event
}

func (f *fakePublisher) Publish(ctx context.Context, e domain.Event) error {
	f.published = append(f.published, e)
	return nil
}

func (f *fakePublisher) Close(ctx context.Context) error { return nil }

func TestNewUseCase_SetsFields(t *testing.T) {
	pub := &nopPublisher{}
	mf := 10 * time.Second
	mp := 24 * time.Hour
	uc := NewUseCase(pub, mf, mp)

	if uc.publisher == nil {
		t.Fatalf("publisher is nil")
	}
	if got, ok := uc.publisher.(*nopPublisher); !ok || got != pub {
		t.Fatalf("expected publisher to be set to nopPublisher instance")
	}
	if uc.maxFuture != mf {
		t.Fatalf("expected maxFuture %v, got %v", mf, uc.maxFuture)
	}
	if uc.maxPast != mp {
		t.Fatalf("expected maxPast %v, got %v", mp, uc.maxPast)
	}
}

func TestEventPublisherImplementation(t *testing.T) {
	// Compile-time assertion: fakePublisher implements EventPublisher.
	var _ EventPublisher = (*fakePublisher)(nil)
}

func TestUseCase_PublisherCalled(t *testing.T) {
	fp := &fakePublisher{}
	uc := &UseCase{
		publisher: fp,
		maxFuture: time.Hour * 24,
		maxPast:   time.Hour * 24 * 365,
	}

	// call publisher directly via the injected seam to verify wiring works.
	ev := domain.Event{
		EventName:   "e1",
		UserID:      "u1",
		TimestampMs: uint64(time.Now().UnixMilli()),
		Channel:     "",
		CampaignID:  "",
		Tags:        []string{},
		Metadata:    "{}",
		PayloadHash: 0,
	}
	if err := uc.publisher.Publish(context.Background(), ev); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if len(fp.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(fp.published))
	}
}

// --- Ingest use-case behavior tests ---

func TestIngest_ValidEvent(t *testing.T) {
	fp := &fakePublisher{}
	uc := NewUseCase(fp, time.Hour, 24*time.Hour)

	raw := &domain.RawEvent{
		EventName:  "product_view",
		UserID:     "user1",
		Timestamp:  time.Now().Unix(), // seconds -> normalized to ms
		Channel:    "web",
		CampaignID: "cmp",
		Tags:       []string{"a", "b"},
	}

	if err := uc.Ingest(context.Background(), raw); err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}
	if len(fp.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(fp.published))
	}
	p := fp.published[0]
	if p.EventName != raw.EventName || p.UserID != raw.UserID {
		t.Fatalf("published event mismatch: got %+v", p)
	}
	if p.TimestampMs == 0 {
		t.Fatalf("expected normalized timestamp_ms to be set")
	}
	if p.PayloadHash == 0 {
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
		EventName: "e",
		UserID:    "u",
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
		EventName: "evt",
		UserID:    "u",
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
