package ingest

import (
	"testing"
	"time"

	"eventmetrics/internal/domain"
)

type nopPublisher struct{}

func (n *nopPublisher) Publish(ctx context.Context, e domain.Event) error { return nil }
func (n *nopPublisher) Close(ctx context.Context) error                 { return nil }

func TestNewUseCase_SetsFields(t *testing.T) {
	pub := &nopPublisher{}
	mf := 10 * time.Second
	mp := 24 * time.Hour
	uc := NewUseCase(pub, mf, mp)

	if uc.publisher != pub {
		t.Fatalf("expected publisher set")
	}
	if uc.maxFuture != mf {
		t.Fatalf("expected maxFuture %v, got %v", mf, uc.maxFuture)
	}
	if uc.maxPast != mp {
		t.Fatalf("expected maxPast %v, got %v", mp, uc.maxPast)
	}
}

