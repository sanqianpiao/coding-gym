-- Drop outbox table
DROP INDEX IF EXISTS idx_outbox_topic;
DROP INDEX IF EXISTS idx_outbox_created_at;
DROP INDEX IF EXISTS idx_outbox_aggregate;
DROP INDEX IF EXISTS idx_outbox_status_created_at;
DROP TABLE IF EXISTS outbox_events;