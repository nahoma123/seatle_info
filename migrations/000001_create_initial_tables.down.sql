-- File: migrations/000001_create_initial_tables.down.sql

-- Drop triggers first if they exist
DROP TRIGGER IF EXISTS set_timestamp_app_configurations ON app_configurations;
DROP TRIGGER IF EXISTS set_timestamp_listings ON listings;
DROP TRIGGER IF EXISTS set_timestamp_sub_categories ON sub_categories;
DROP TRIGGER IF EXISTS set_timestamp_categories ON categories;
DROP TRIGGER IF EXISTS set_timestamp_users ON users;

-- Drop the function
DROP FUNCTION IF EXISTS trigger_set_timestamp();

-- Drop tables in reverse order of creation due to foreign key constraints
DROP TABLE IF EXISTS app_configurations;
DROP TABLE IF EXISTS listing_details_events;
DROP TABLE IF EXISTS listing_details_housing;
DROP TABLE IF EXISTS listing_details_babysitting;
DROP TABLE IF EXISTS listings;
DROP TABLE IF EXISTS sub_categories;
DROP TABLE IF EXISTS categories;
DROP TABLE IF EXISTS users;

-- Optionally, drop extensions if they are not used by other parts of the database.
-- Be cautious with this in a shared database.
-- DROP EXTENSION IF EXISTS postgis;
-- DROP EXTENSION IF EXISTS "uuid-ossp";