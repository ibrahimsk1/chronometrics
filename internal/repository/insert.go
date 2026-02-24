package repository

import (
	"context"

	"eventmetrics/internal/domain"
)

// Flush writes a batch of events to ClickHouse using the batch API.
func (r *Repository) Flush(ctx context.Context, batchEvents []domain.Event) error {
	if len(batchEvents) == 0 {
		return nil
	}
	batch, err := r.conn.PrepareBatch(ctx, "INSERT INTO events (event_name, user_id, timestamp_ms, payload_hash, channel, campaign_id, tags, metadata) VALUES")
	if err != nil {
		return err
	}
	for _, e := range batchEvents {
		if err := batch.Append(
			e.EventName,
			e.UserID,
			e.TimestampMs,
			e.PayloadHash,
			e.Channel,
			e.CampaignID,
			e.Tags,
			e.Metadata,
		); err != nil {
			_ = batch.Abort()
			return err
		}
	}
	return batch.Send()
}

