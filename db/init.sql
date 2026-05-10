CREATE TABLE IF NOT EXISTS received_messages (
    id BIGSERIAL PRIMARY KEY,
    message_id VARCHAR(100) NOT NULL UNIQUE,
    payload JSONB NOT NULL,
    kafka_topic TEXT NOT NULL DEFAULT '',
    kafka_partition INTEGER NOT NULL DEFAULT 0,
    kafka_offset BIGINT NOT NULL DEFAULT 0,
    producer_timestamp TIMESTAMPTZ,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    duplicate_count BIGINT NOT NULL DEFAULT 0
);

ALTER TABLE received_messages
    ADD COLUMN IF NOT EXISTS kafka_topic TEXT NOT NULL DEFAULT '';

ALTER TABLE received_messages
    ADD COLUMN IF NOT EXISTS kafka_partition INTEGER NOT NULL DEFAULT 0;

ALTER TABLE received_messages
    ADD COLUMN IF NOT EXISTS kafka_offset BIGINT NOT NULL DEFAULT 0;

ALTER TABLE received_messages
    ADD COLUMN IF NOT EXISTS producer_timestamp TIMESTAMPTZ;

ALTER TABLE received_messages
    ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

ALTER TABLE received_messages
    ADD COLUMN IF NOT EXISTS duplicate_count BIGINT NOT NULL DEFAULT 0;

UPDATE received_messages
SET last_seen_at = received_at
WHERE last_seen_at IS NULL;

ALTER TABLE received_messages
    DROP COLUMN IF EXISTS schema_version;
