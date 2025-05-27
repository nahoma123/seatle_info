-- migrations/seed.sql
-- This file contains sample data to be loaded into the database for development.
-- Populate this file with INSERT statements for your tables.

-- Example for a 'users' table:
-- INSERT INTO users (id, username, email, password_hash, created_at, updated_at) VALUES
-- (gen_random_uuid(), 'devuser', 'dev@example.com', 'hashed_password_placeholder', NOW(), NOW()),
-- (gen_random_uuid(), 'testuser', 'test@example.com', 'hashed_password_placeholder', NOW(), NOW());

-- Example for a 'categories' table:
-- INSERT INTO categories (id, name, created_at, updated_at) VALUES
-- (gen_random_uuid(), 'Electronics', NOW(), NOW()),
-- (gen_random_uuid(), 'Books', NOW(), NOW()),
-- (gen_random_uuid(), 'Furniture', NOW(), NOW());

-- Example for a 'listings' table (assuming it has user_id and category_id as foreign keys):
-- INSERT INTO listings (id, user_id, category_id, title, description, price, location, created_at, updated_at, status) VALUES
-- (gen_random_uuid(), (SELECT id from users WHERE username = 'devuser'), (SELECT id from categories WHERE name = 'Electronics'), 'Sample Listing 1', 'Description for listing 1', 100.00, 'Sample Location 1', NOW(), NOW(), 'active'),
-- (gen_random_uuid(), (SELECT id from users WHERE username = 'testuser'), (SELECT id from categories WHERE name = 'Books'), 'Sample Listing 2', 'Description for listing 2', 25.50, 'Sample Location 2', NOW(), NOW(), 'active');

-- Add more INSERT statements for other tables as needed.
-- Remember to handle dependencies (e.g., insert users before listings that reference users).
-- Use appropriate functions for generating IDs (e.g., gen_random_uuid() for UUIDs) if applicable.
