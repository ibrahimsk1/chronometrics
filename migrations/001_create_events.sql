-- migrations/001_create_events.sql
CREATE TABLE IF NOT EXISTS events (
    event_name   String,
    user_id      String,
    timestamp_ms UInt64,
    payload_hash UInt64,
    channel      String DEFAULT '',
    campaign_id  String DEFAULT '',
    tags         Array(String) DEFAULT [],
    metadata     String DEFAULT '',
    _inserted_at DateTime DEFAULT now()
) ENGINE = ReplacingMergeTree(_inserted_at)
ORDER BY (event_name, user_id, timestamp_ms, payload_hash)
PARTITION BY toYYYYMM(toDateTime(intDiv(timestamp_ms, 1000)));

