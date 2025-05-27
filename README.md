# File: migrations/README.md
# Database Migrations

This directory contains database migration files managed by [golang-migrate/migrate](https://github.com/golang-migrate/migrate).

## Prerequisites

*   Install the `migrate` CLI. See [installation instructions](https://github.com/golang-migrate/migrate/tree/master/cmd/migrate).
*   Ensure your database server (e.g., PostgreSQL) is running.
*   Set the `DB_SOURCE` environment variable. This should be a DSN URL format that `golang-migrate` understands.
    Example for PostgreSQL:
    ```bash
    export DB_SOURCE="postgresql://your_user:your_password@localhost:5432/seattle_info_db?sslmode=disable"
    ```
    You can put this in your `.env` file, and your shell environment should pick it up if you `source` it or if your `Makefile` exports it.

## Usage

You can use the `Makefile` targets for common migration tasks:

*   **Apply all pending up migrations:**
    ```bash
    make migrate-up
    ```

*   **Revert the last applied migration:**
    ```bash
    make migrate-down
    ```
    To revert more than one, you can run `migrate -database "$DB_SOURCE" -path ./migrations down N` where N is the number of migrations to revert.

*   **Create a new migration:**
    Replace `<migration_name>` with a descriptive name for your migration (e.g., `add_user_profiles`).
    ```bash
    make migrate-create NAME=my_new_migration
    ```
    This will create two files: `timestamp_my_new_migration.up.sql` and `timestamp_my_new_migration.down.sql`.

## Important Notes

*   Always write both `up` and `down` migrations.
*   Test your migrations thoroughly in a development environment before applying them to staging or production.
*   Once a migration has been applied to a shared environment (staging, production), **do not edit it**. If changes are needed, create a new migration to modify the schema.