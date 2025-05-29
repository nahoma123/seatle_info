-- Remove the firebase_uid column
-- The unique index idx_users_firebase_uid will be dropped automatically in PostgreSQL
ALTER TABLE users DROP COLUMN firebase_uid;

-- Note: The provider_id column was already nullable, so no ALTER COLUMN ... SET NOT NULL
-- statement is needed here to revert its nullability.
