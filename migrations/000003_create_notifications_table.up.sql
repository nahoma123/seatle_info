-- File: migrations/000003_create_notifications_table.up.sql

-- Ensure uuid-ossp extension is available (usually enabled in an earlier migration)
-- CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS notifications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type VARCHAR(100) NOT NULL,
    message TEXT NOT NULL,
    related_listing_id UUID REFERENCES listings(id) ON DELETE SET NULL,
    is_read BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
    -- No updated_at as notifications are generally immutable
);

-- Index for efficient fetching of user notifications, ordered by creation time, and filtered by read status.
CREATE INDEX IF NOT EXISTS idx_notifications_user_read_created ON notifications(user_id, is_read, created_at DESC);

-- Optional: Index on related_listing_id if you frequently query notifications by listing
CREATE INDEX IF NOT EXISTS idx_notifications_related_listing_id ON notifications(related_listing_id);
