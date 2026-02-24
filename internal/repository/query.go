package repository

import (
	"context"
	"fmt"

	"eventmetrics/internal/domain"
)

// Query executes aggregation queries against ClickHouse and returns MetricResult.
func (r *Repository) Query(ctx context.Context, params domain.QueryParams) (*domain.MetricResult, error) {
	// enforce configured timeout
	qctx, cancel := context.WithTimeout(ctx, r.cfg.QueryTimeout)
	defer cancel()

	// Totals query
	totalsSQL := `
SELECT
  count() AS total_count,
  uniqExact(user_id) AS unique_count
FROM events FINAL
WHERE event_name = ? AND timestamp_ms >= ? AND timestamp_ms < ?`

	rows, err := r.conn.Query(qctx, totalsSQL, params.EventName, params.From, params.To)
	if err != nil {
		if qctx.Err() == context.DeadlineExceeded {
			return nil, domain.ErrQueryTimeout
		}
		return nil, fmt.Errorf("totals query: %w", err)
	}
	defer rows.Close()

	var totalCount uint64
	var uniqueCount uint64
	if rows.Next() {
		if err := rows.Scan(&totalCount, &uniqueCount); err != nil {
			return nil, fmt.Errorf("scan totals: %w", err)
		}
	}

	res := &domain.MetricResult{
		EventName:   params.EventName,
		From:        params.From,
		To:          params.To,
		TotalCount:  totalCount,
		UniqueCount: uniqueCount,
		GroupBy:     params.GroupBy,
		Groups:      []domain.GroupResult{},
	}

	// If grouping requested, run group query
	if params.GroupBy != "" {
		var groupSQL string
		switch params.GroupBy {
		case "channel":
			groupSQL = `
SELECT channel AS key, count() AS total_count, uniqExact(user_id) AS unique_count
FROM events FINAL
WHERE event_name = ? AND timestamp_ms >= ? AND timestamp_ms < ?
GROUP BY channel
ORDER BY total_count DESC`
		case "hour":
			groupSQL = `
SELECT toISO8601(toStartOfHour(toDateTime(intDiv(timestamp_ms,1000)))) AS key, count() AS total_count, uniqExact(user_id) AS unique_count
FROM events FINAL
WHERE event_name = ? AND timestamp_ms >= ? AND timestamp_ms < ?
GROUP BY toStartOfHour(toDateTime(intDiv(timestamp_ms,1000)))
ORDER BY key`
		case "day":
			groupSQL = `
SELECT toISO8601(toDate(toDateTime(intDiv(timestamp_ms,1000)))) AS key, count() AS total_count, uniqExact(user_id) AS unique_count
FROM events FINAL
WHERE event_name = ? AND timestamp_ms >= ? AND timestamp_ms < ?
GROUP BY toDate(toDateTime(intDiv(timestamp_ms,1000)))
ORDER BY key`
		default:
			return nil, fmt.Errorf("unsupported group_by: %q", params.GroupBy)
		}

		grows, err := r.conn.Query(qctx, groupSQL, params.EventName, params.From, params.To)
		if err != nil {
			if qctx.Err() == context.DeadlineExceeded {
				return nil, domain.ErrQueryTimeout
			}
			return nil, fmt.Errorf("group query: %w", err)
		}
		defer grows.Close()

		for grows.Next() {
			var key string
			var tcnt uint64
			var ucnt uint64
			if err := grows.Scan(&key, &tcnt, &ucnt); err != nil {
				return nil, fmt.Errorf("scan group row: %w", err)
			}
			res.Groups = append(res.Groups, domain.GroupResult{
				Key:         key,
				TotalCount:  tcnt,
				UniqueCount: ucnt,
			})
		}
	}

	return res, nil
}

