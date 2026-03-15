package repository

import (
	"context"
	"fmt"
	"sync"
	"time"

	"eventmetrics/internal/domain"
)

// Query executes aggregation queries against ClickHouse and returns MetricResult.
func (r *Repository) Query(ctx context.Context, params domain.QueryParams) (*domain.MetricResult, error) {
	// enforce configured timeout
	qctx, cancel := context.WithTimeout(ctx, r.cfg.QueryTimeout)
	defer cancel()
	start := time.Now()
	defer func() {
		if r.metrics != nil {
			result := "success"
			if qctx.Err() != nil {
				result = "error"
			}
			r.metrics.DBQueryDuration.WithLabelValues(result).Observe(time.Since(start).Seconds())
		}
	}()

	// Totals query
	totalsSQL := `
		SELECT count() AS total_count, uniqExact(user_id) AS unique_count
		FROM events FINAL
		WHERE event_name = ? AND timestamp_ms >= ? AND timestamp_ms < ?
	`

	rows, err := r.conn.Query(qctx, totalsSQL, params.EventName, params.From, params.To)
	if err != nil {
		if qctx.Err() == context.DeadlineExceeded {
			return nil, domain.ErrQueryTimeout
		}
		return nil, fmt.Errorf("totals query: %w", err)
	}
	defer func() { _ = rows.Close() }()

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

	// switch case ile yazılmış. group by is parametize edilmemiş.
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
				ORDER BY total_count DESC
			`
		case "campaign_id":
			groupSQL = `
				SELECT campaign_id as key, count() as total_count, uniqExact(user_id) AS uniqie_count
				From events FINAL
				WHERE event_name = ? AND timestamp_ms >= ? AND timestamp_ms < ?
				GROUP BY campaign_id
				ORDER BY total_count DESC
			`
		case "hour":
			groupSQL = `
				SELECT toISO8601(toStartOfHour(toDateTime(intDiv(timestamp_ms,1000)))) AS key, count() AS total_count, uniqExact(user_id) AS unique_count
				FROM events FINAL
				WHERE event_name = ? AND timestamp_ms >= ? AND timestamp_ms < ?
				GROUP BY toStartOfHour(toDateTime(intDiv(timestamp_ms,1000)))
				ORDER BY key
			`
		case "day":
			groupSQL = `
				SELECT toISO8601(toDate(toDateTime(intDiv(timestamp_ms,1000)))) AS key, count() AS total_count, uniqExact(user_id) AS unique_count
				FROM events FINAL
				WHERE event_name = ? AND timestamp_ms >= ? AND timestamp_ms < ?
				GROUP BY toDate(toDateTime(intDiv(timestamp_ms,1000)))
				ORDER BY key
			`
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
		defer func() { _ = grows.Close() }()

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

var (
	once            sync.Once
	lookupDimension func(string) string
)

func get(key string) string {
	once.Do(func() {
		m := map[string]string{
			"a": "b",
		}
		lookupDimension = func(key string) string {
			return m[key]
		}
	})
	return lookupDimension(key)
}
