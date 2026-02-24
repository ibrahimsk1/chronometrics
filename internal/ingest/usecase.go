package ingest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"eventmetrics/internal/domain"
)

// EventPublisher is the strategy seam used by the ingest use-case.
// Implementation: internal/buffer.Buffer (memory).
type EventPublisher interface {
	Publish(ctx context.Context, event domain.Event) error
	Close(ctx context.Context) error
}

// UseCase orchestrates event ingestion: validation and publish.
type UseCase struct {
	publisher EventPublisher
	maxFuture time.Duration
	maxPast   time.Duration
}

// NewUseCase constructs a UseCase with the provided publisher and validation windows.
func NewUseCase(pub EventPublisher, maxFuture, maxPast time.Duration) *UseCase {
	return &UseCase{
		publisher: pub,
		maxFuture: maxFuture,
		maxPast:   maxPast,
	}
}

// Ingest validates, normalizes, and publishes a RawEvent.
func (uc *UseCase) Ingest(ctx context.Context, raw *domain.RawEvent) error {
	if raw == nil {
		return fmt.Errorf("raw event is nil")
	}
	if err := domain.Validate(raw, uc.maxFuture, uc.maxPast); err != nil {
		return err
	}
	ev, err := domain.ToEvent(raw)
	if err != nil {
		return err
	}
	if err := uc.publisher.Publish(ctx, ev); err != nil {
		// Wrap publisher error with the domain sentinel while preserving original cause.
		// This ensures errors.Is(err, domain.ErrPublishFailed) will be true.
		return fmt.Errorf("publish failed: %w", errors.Join(domain.ErrPublishFailed, err))
	}
	return nil
}
