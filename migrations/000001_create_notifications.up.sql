CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id UUID,
    idempotency_key VARCHAR(255),
    recipient VARCHAR(255) NOT NULL,
    channel VARCHAR(10) NOT NULL CHECK (channel IN ('sms', 'email', 'push')),
    content TEXT NOT NULL,
    priority VARCHAR(10) NOT NULL DEFAULT 'normal' CHECK (priority IN ('high', 'normal', 'low')),
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'sent', 'failed', 'cancelled')),
    provider_message_id VARCHAR(255),
    retry_count INTEGER NOT NULL DEFAULT 0,
    error_message TEXT,
    correlation_id VARCHAR(255) NOT NULL,
    template_vars TEXT,
    scheduled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ
);

-- Unique index for idempotency
CREATE UNIQUE INDEX idx_notifications_idempotency_key ON notifications (idempotency_key) WHERE idempotency_key IS NOT NULL;

-- Composite indexes for query patterns
CREATE INDEX idx_notifications_status_channel ON notifications (status, channel);
CREATE INDEX idx_notifications_cursor ON notifications (created_at DESC, id DESC);
CREATE INDEX idx_notifications_batch ON notifications (batch_id, created_at) WHERE batch_id IS NOT NULL;
CREATE INDEX idx_notifications_scheduled ON notifications (scheduled_at) WHERE scheduled_at IS NOT NULL;
