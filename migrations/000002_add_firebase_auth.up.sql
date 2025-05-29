-- Add firebase_uid column to store Firebase User ID
ALTER TABLE users ADD COLUMN firebase_uid VARCHAR(255) NULL;

-- Create a unique index on firebase_uid for efficient lookups
CREATE UNIQUE INDEX idx_users_firebase_uid ON users (firebase_uid);

-- Add a comment to the column for clarity
COMMENT ON COLUMN users.firebase_uid IS 'Firebase User ID';

-- Note: The provider_id column was found to be already nullable in the initial schema,
-- so no ALTER COLUMN ... DROP NOT NULL statement is needed for it here.
