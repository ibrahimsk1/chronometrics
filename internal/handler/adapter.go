package handler

import (
	"context"

	"eventmetrics/internal/domain"
	"eventmetrics/internal/ingest"
)

// UseCaseAdapter adapts the ingest.UseCase to the handler.Ingester interface.
type UseCaseAdapter struct {
	uc *ingest.UseCase
}

// NewUseCaseAdapter constructs an adapter for the given use case.
func NewUseCaseAdapter(uc *ingest.UseCase) *UseCaseAdapter {
	return &UseCaseAdapter{uc: uc}
}

// Ingest delegates to the underlying use case.
func (a *UseCaseAdapter) Ingest(ctx context.Context, e *domain.RawEvent) error {
	return a.uc.Ingest(ctx, e)
}
