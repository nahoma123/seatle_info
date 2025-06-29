-- Enable UUID generation if not already enabled (idempotent)
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Listing Images Table
CREATE TABLE IF NOT EXISTS listing_images (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    listing_id UUID NOT NULL REFERENCES listings(id) ON DELETE CASCADE,
    image_path TEXT NOT NULL, -- Relative path to the image file from the configured image storage root
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP -- Added for consistency, will need trigger
);

-- Indexes for listing_images
CREATE INDEX IF NOT EXISTS idx_listing_images_listing_id ON listing_images(listing_id);

-- Apply the existing trigger function for 'updated_at' timestamp
-- Ensure the trigger function 'trigger_set_timestamp()' from 000001 migration is available.
CREATE TRIGGER set_timestamp_listing_images
BEFORE UPDATE ON listing_images
FOR EACH ROW
EXECUTE FUNCTION trigger_set_timestamp();
