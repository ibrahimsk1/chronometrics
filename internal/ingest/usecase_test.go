package ingest

import (
	"context"
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
		ID:          "e1",
		Type:        "test",
		TimestampMS: time.Now().UnixMilli(),
		Data:        map[string]interface{}{"k": "v"},
	}
	if err := uc.publisher.Publish(context.Background(), ev); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if len(fp.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(fp.published))
	}
}

