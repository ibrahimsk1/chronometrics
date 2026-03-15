-- migrations/002_add_priority_to_events.sql
ALTER TABLE events ADD COLUMN IF NOT EXISTS priority Nullable(String);
