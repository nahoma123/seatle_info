-- File: migrations/000003_create_notifications_table.down.sql

DROP INDEX IF EXISTS idx_notifications_related_listing_id;
DROP INDEX IF EXISTS idx_notifications_user_read_created;
DROP TABLE IF EXISTS notifications;
