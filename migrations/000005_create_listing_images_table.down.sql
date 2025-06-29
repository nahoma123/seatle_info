-- Drop the trigger from the listing_images table
DROP TRIGGER IF EXISTS set_timestamp_listing_images ON listing_images;

-- Drop Indexes for listing_images
DROP INDEX IF EXISTS idx_listing_images_listing_id;

-- Drop the Listing Images Table
DROP TABLE IF EXISTS listing_images;
