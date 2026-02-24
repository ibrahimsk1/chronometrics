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

// Ingest converts handler.RawEvent to domain.RawEvent and delegates to use case.
func (a *UseCaseAdapter) Ingest(ctx context.Context, e *RawEvent) error {
	dr := &domain.RawEvent{
		EventName:  e.EventName,
		UserID:     e.UserID,
		Timestamp:  e.Timestamp,
		Channel:    e.Channel,
		CampaignID: e.CampaignID,
		Tags:       e.Tags,
		Metadata:   e.Metadata,
	}
	return a.uc.Ingest(ctx, dr)
}
