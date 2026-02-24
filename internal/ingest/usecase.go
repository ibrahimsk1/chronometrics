package ingest

import (
	"context"
	"time"

	"eventmetrics/internal/domain"
)

// EventPublisher is the strategy seam used by the ingest use-case.
// Implementations: internal/buffer.Buffer (memory) and internal/broker.Publisher (nats).
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

