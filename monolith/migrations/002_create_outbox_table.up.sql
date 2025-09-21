-- Create outbox table for reliable message delivery
CREATE TABLE IF NOT EXISTS outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type VARCHAR(255) NOT NULL,
    aggregate_id VARCHAR(255) NOT NULL,
    event_type VARCHAR(255) NOT NULL,
    event_data JSONB NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'NEW',
    topic VARCHAR(255) NOT NULL,
    partition_key VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    processed_at TIMESTAMP WITH TIME ZONE,
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3,
    error_message TEXT,
    
    CONSTRAINT chk_status CHECK (status IN ('NEW', 'PROCESSING', 'SENT', 'FAILED'))
);

-- Create indexes for efficient polling and processing
CREATE INDEX IF NOT EXISTS idx_outbox_status_created_at ON outbox_events(status, created_at) 
WHERE status = 'NEW';

CREATE INDEX IF NOT EXISTS idx_outbox_aggregate ON outbox_events(aggregate_type, aggregate_id);

CREATE INDEX IF NOT EXISTS idx_outbox_created_at ON outbox_events(created_at);

CREATE INDEX IF NOT EXISTS idx_outbox_topic ON outbox_events(topic);