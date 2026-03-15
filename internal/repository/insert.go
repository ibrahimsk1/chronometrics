package repository

import (
	"context"
	"time"

	"eventmetrics/internal/domain"
)

// Flush writes a batch of events to ClickHouse using the batch API.
func (r *Repository) Flush(ctx context.Context, batchEvents []domain.Event) error {
	if len(batchEvents) == 0 {
		return nil
	}
	start := time.Now()
	batch, err := r.conn.PrepareBatch(ctx, "INSERT INTO events (event_name, user_id, timestamp_ms, payload_hash, channel, campaign_id, tags, priority, metadata) VALUES")
	if err != nil {
		if r.metrics != nil {
			r.metrics.DBInsertDuration.WithLabelValues("error").Observe(time.Since(start).Seconds())
		}
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
			e.Priority,
			e.Metadata,
		); err != nil {
			_ = batch.Abort()
			if r.metrics != nil {
				r.metrics.DBInsertDuration.WithLabelValues("error").Observe(time.Since(start).Seconds())
			}
			return err
		}
	}
	err = batch.Send()
	if r.metrics != nil {
		result := "success"
		if err != nil {
			result = "error"
		}
		r.metrics.DBInsertDuration.WithLabelValues(result).Observe(time.Since(start).Seconds())
	}
	if err != nil {
		r.logger.Error("flush failed", "batch_size", len(batchEvents), "error", err)
	}
	return err
}
