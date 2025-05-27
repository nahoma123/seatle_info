-- File: migrations/000001_create_initial_tables.up.sql

-- Enable UUID generation
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Enable PostGIS for geospatial capabilities
CREATE EXTENSION IF NOT EXISTS postgis;

-- Users Table
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) UNIQUE, -- Nullable for OAuth users who might not provide email initially
    password_hash VARCHAR(255), -- Nullable for OAuth-only users
    first_name VARCHAR(100),
    last_name VARCHAR(100),
    profile_picture_url TEXT,
    auth_provider VARCHAR(50) NOT NULL DEFAULT 'email', -- 'email', 'google', 'apple'
    provider_id VARCHAR(255), -- Unique ID from the OAuth provider
    is_email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    role VARCHAR(50) NOT NULL DEFAULT 'user', -- 'user', 'admin'
    is_first_post_approved BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_login_at TIMESTAMPTZ,
    CONSTRAINT unique_provider_id_per_provider UNIQUE (auth_provider, provider_id)
);

-- Index for faster email lookups
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_auth_provider_provider_id ON users(auth_provider, provider_id);


-- Categories Table (BR1.1)
CREATE TABLE IF NOT EXISTS categories (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL UNIQUE,
    slug VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- SubCategories Table (BR1.2 - e.g., for Business)
CREATE TABLE IF NOT EXISTS sub_categories (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    slug VARCHAR(100) NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (category_id, name), -- Subcategory name unique within a category
    UNIQUE (category_id, slug)  -- Subcategory slug unique within a category
);
CREATE INDEX IF NOT EXISTS idx_sub_categories_category_id ON sub_categories(category_id);


-- Listings Table (BR1 general, TS2.4)
CREATE TABLE IF NOT EXISTS listings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    sub_category_id UUID REFERENCES sub_categories(id) ON DELETE SET NULL, -- Nullable
    title VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active', -- 'pending_approval', 'active', 'expired', 'rejected', 'admin_removed'
    contact_name VARCHAR(150),
    contact_email VARCHAR(255),
    contact_phone VARCHAR(50),
    address_line1 VARCHAR(255),
    address_line2 VARCHAR(255),
    city VARCHAR(100) DEFAULT 'Seattle',
    state VARCHAR(50) DEFAULT 'WA',
    zip_code VARCHAR(20),
    -- Storing latitude and longitude separately for direct access if needed,
    -- but primary geospatial operations will use the 'location' field.
    latitude DECIMAL(10, 8),
    longitude DECIMAL(11, 8),
    location GEOGRAPHY(Point, 4326), -- PostGIS geography type for lat/lon points (SRID 4326 is WGS 84)
    expires_at TIMESTAMPTZ NOT NULL,
    is_admin_approved BOOLEAN NOT NULL DEFAULT FALSE, -- For BR3.3 (first post moderation)
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT check_lat_lon_both_null_or_not_null CHECK ((latitude IS NULL AND longitude IS NULL AND location IS NULL) OR (latitude IS NOT NULL AND longitude IS NOT NULL AND location IS NOT NULL))
);

-- Indexes for listings
CREATE INDEX IF NOT EXISTS idx_listings_user_id ON listings(user_id);
CREATE INDEX IF NOT EXISTS idx_listings_category_id ON listings(category_id);
CREATE INDEX IF NOT EXISTS idx_listings_sub_category_id ON listings(sub_category_id);
CREATE INDEX IF NOT EXISTS idx_listings_status ON listings(status);
CREATE INDEX IF NOT EXISTS idx_listings_expires_at ON listings(expires_at);
-- Geospatial index for efficient location-based queries
CREATE INDEX IF NOT EXISTS idx_listings_location ON listings USING GIST (location);


-- Listing Details: Baby Sitting (BR1.3)
CREATE TABLE IF NOT EXISTS listing_details_babysitting (
    listing_id UUID PRIMARY KEY REFERENCES listings(id) ON DELETE CASCADE,
    languages_spoken TEXT[] -- Array of strings, e.g., '{"English", "Amharic", "Spanish"}'
);

-- Listing Details: Housing (BR1.4)
CREATE TABLE IF NOT EXISTS listing_details_housing (
    listing_id UUID PRIMARY KEY REFERENCES listings(id) ON DELETE CASCADE,
    property_type VARCHAR(50) NOT NULL, -- e.g., 'for_rent', 'for_sale'
    rent_details VARCHAR(255), -- e.g., '1-bedroom', 'subletting', 'studio' (for 'for_rent' type)
    sale_price NUMERIC(12, 2) -- For 'for_sale' type
);
CREATE INDEX IF NOT EXISTS idx_listing_details_housing_property_type ON listing_details_housing(property_type);


-- Listing Details: Events (BR1.5)
CREATE TABLE IF NOT EXISTS listing_details_events (
    listing_id UUID PRIMARY KEY REFERENCES listings(id) ON DELETE CASCADE,
    event_date DATE NOT NULL,
    event_time TIME, -- Nullable if it's an all-day event or time is TBD
    organizer_name VARCHAR(150),
    venue_name VARCHAR(255) -- Could also be part of the main address fields in listings
);
CREATE INDEX IF NOT EXISTS idx_listing_details_events_event_date ON listing_details_events(event_date);


-- Application Configurations Table (BR2.2, BR2.3, BR3.3)
CREATE TABLE IF NOT EXISTS app_configurations (
    key VARCHAR(100) PRIMARY KEY,
    value TEXT NOT NULL,
    description TEXT,
    data_type VARCHAR(50) DEFAULT 'string', -- 'string', 'integer', 'boolean', 'date'
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Pre-populate some configurations
INSERT INTO app_configurations (key, value, description, data_type) VALUES
('MAX_LISTING_DISTANCE_KM', '50', 'Maximum distance (in KM) for location-based filtering of listings.', 'integer')
ON CONFLICT (key) DO NOTHING;

INSERT INTO app_configurations (key, value, description, data_type) VALUES
('DEFAULT_LISTING_LIFESPAN_DAYS', '10', 'Default lifespan (in days) for new listings before they auto-expire.', 'integer')
ON CONFLICT (key) DO NOTHING;

INSERT INTO app_configurations (key, value, description, data_type) VALUES
('FIRST_POST_APPROVAL_MODEL_ACTIVE_UNTIL', (CURRENT_DATE + INTERVAL '6 months')::text, 'Date until which the first-post approval model is active. After this date, new users first posts are auto-approved.', 'date')
ON CONFLICT (key) DO NOTHING;


-- Function to automatically update 'updated_at' timestamp
CREATE OR REPLACE FUNCTION trigger_set_timestamp()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply the trigger to tables that have 'updated_at'
CREATE TRIGGER set_timestamp_users
BEFORE UPDATE ON users
FOR EACH ROW
EXECUTE FUNCTION trigger_set_timestamp();

CREATE TRIGGER set_timestamp_categories
BEFORE UPDATE ON categories
FOR EACH ROW
EXECUTE FUNCTION trigger_set_timestamp();

CREATE TRIGGER set_timestamp_sub_categories
BEFORE UPDATE ON sub_categories
FOR EACH ROW
EXECUTE FUNCTION trigger_set_timestamp();

CREATE TRIGGER set_timestamp_listings
BEFORE UPDATE ON listings
FOR EACH ROW
EXECUTE FUNCTION trigger_set_timestamp();

CREATE TRIGGER set_timestamp_app_configurations
BEFORE UPDATE ON app_configurations
FOR EACH ROW
EXECUTE FUNCTION trigger_set_timestamp();

-- TODO: Add initial data for categories if needed (e.g., Business, Housing, Events, Baby Sitting)
INSERT INTO categories (name, slug, description) VALUES
('Businesses', 'businesses', 'Local Habesha businesses and services.'),
('Baby Sitting', 'baby-sitting', 'Find or offer baby sitting services.'),
('Events', 'events', 'Community events, gatherings, and announcements.'),
('Housing', 'housing', 'Property for rent or sale.')
ON CONFLICT (slug) DO NOTHING;

-- Optionally, add some subcategories for 'Businesses'
WITH business_category AS (
    SELECT id FROM categories WHERE slug = 'businesses'
)
INSERT INTO sub_categories (category_id, name, slug, description) VALUES
((SELECT id FROM business_category), 'Restaurants & Cafes', 'restaurants-cafes', 'Places to eat and drink.'),
((SELECT id FROM business_category), 'Legal Services', 'legal-services', 'Legal advice and representation.'),
((SELECT id FROM business_category), 'Retail Shops', 'retail-shops', 'Local shops and stores.'),
((SELECT id FROM business_category), 'Hair & Beauty Salons', 'hair-beauty-salons', 'Salons and barber shops.'),
((SELECT id FROM business_category), 'Professional Services', 'professional-services', 'Accounting, consulting, etc.')
ON CONFLICT (category_id, slug) DO NOTHING;
