# Seattle Info Backend

This project is a backend application for providing information related to Seattle.

## Prerequisites

Before you begin, ensure you have the following installed:

*   **Go**: Version 1.21 or higher.
*   **Docker**: For running development and production-like environments.
*   **Docker Compose**: For managing multi-container Docker applications.
*   **golang-migrate/migrate CLI**: For database migrations. See [installation instructions](https://github.com/golang-migrate/migrate/tree/master/cmd/migrate).
*   **make**: For using the Makefile commands.

## Environment Configuration

The application uses `.env` files for managing environment-specific configurations. These files are gitignored for security.

1.  **Copy the Example:**
    Start by copying the example configuration file:
    ```bash
    cp .env.example .env.development
    cp .env.example .env.production
    ```
    The `make` commands that depend on these files will also guide you if they are missing.

2.  **Customize Configuration:**
    You will need to customize these files for your different environments.

    *   **`.env.development` (For Local Development):**
        *   `DB_USER`, `DB_PASSWORD`, `DB_NAME`: Set these for your local PostgreSQL development database (which will run in Docker).
        *   `DB_HOST`: Should be `localhost` (or `127.0.0.1`) as the Go application runs on your host and connects to the PostgreSQL Docker container's exposed port.
        *   `DB_PORT`: Typically `5432` (this is the host port exposed by the dev DB container).
        *   `DB_SOURCE`: This is for `golang-migrate` when run from your host. It should be a PostgreSQL connection URL pointing to your local development database (e.g., `postgresql://your_dev_user:your_dev_password@localhost:5432/your_dev_db_name?sslmode=disable`).
        *   `OAUTH_COOKIE_DOMAIN`: Set to `localhost`.
        *   `OAUTH_COOKIE_SECURE`: Set to `false`.
        *   Update `GOOGLE_REDIRECT_URI`, `APPLE_REDIRECT_URI` to use `http://localhost:${SERVER_PORT}/...` (SERVER_PORT is also from this file).
        *   You can leave OAuth client IDs/secrets blank if not testing OAuth, or use test credentials.

    *   **`.env.production` (For Production-like Docker Environment):**
        *   `GIN_MODE`: Set to `release`.
        *   `DB_USER`, `DB_PASSWORD`, `DB_NAME`: Set these to the values the `postgres_db` service in `docker-compose.yml` will use to initialize the production database.
        *   `DB_HOST`: This should remain `postgres_db`. This is the service name Docker Compose uses for inter-container communication, allowing the `app` container to find the `postgres_db` container on the internal Docker network.
        *   `DB_PORT`: This should remain `5432` (the internal port of PostgreSQL within the Docker network, used by the `app` container).
        *   `DB_SOURCE`: This variable is used by the Go application *inside* the `app` container. The Go app constructs its DSN from individual DB_* parts, but if it were to use `DB_SOURCE` directly, it should be formatted as `postgresql://your_prod_user:your_prod_password@postgres_db:5432/your_prod_db_name?sslmode=disable`.
        *   **`DB_SOURCE_PROD` (Important for host-based migrations):** This variable is specifically for running `make migrate-up-prod` or `make migrate-down-prod` from your host machine against the production-like database running in Docker. It must use the host-accessible address and the *exposed* port of the production database container (e.g., `postgresql://your_prod_user:your_prod_password@localhost:5433/your_prod_db_name?sslmode=disable`, assuming you've mapped host port 5433 to container port 5432 for the `postgres_db` service in `docker-compose.yml`). If `docker-compose.yml` exposes the prod DB on the same host port as dev (e.g. 5432), ensure only one DB is running at a time to avoid port conflicts.
        *   `JWT_SECRET_KEY`: **Change this to a strong, unique secret.**
        *   Update all OAuth credentials (`GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, etc.) and redirect URIs (`GOOGLE_REDIRECT_URI`, `APPLE_REDIRECT_URI`) to your actual production values (e.g., using `https://yourdomain.com/...`).
        *   `OAUTH_COOKIE_DOMAIN`: Set to your actual production domain.
        *   `OAUTH_COOKIE_SECURE`: Set to `true`.
        *   `SERVER_PORT`: Define the port your production app container will listen on (e.g., 8080), and this will be mapped to a host port in `docker-compose.yml`.

**Note:** The `.env` files are ignored by Git (via `.gitignore`). Only `.env.example` is committed.

## Typical Development Workflow

This workflow uses Docker for the PostgreSQL database and runs the Go application directly on your host machine.

1.  **Configure Environment:**
    Copy `.env.example` to `.env.development` and customize it as described above. If the file is missing, the `make` commands will guide you.

2.  **Start Development Database:**
    ```bash
    make dev-db-up
    ```
    This command starts a PostgreSQL container using `docker-compose.dev.yml` and `.env.development`.

3.  **Apply Database Migrations:**
    ```bash
    make migrate-up
    ```
    This applies all pending migrations to the development database using `DB_SOURCE` from `.env.development`.

4.  **Seed Database (Optional):**
    If you have sample data in `migrations/seed.sql`:
    ```bash
    make seed-dev-db
    ```

5.  **Run the Go Application:**
    ```bash
    make run-dev-app
    ```
    This runs the Go application on your host (using `go run`). It will connect to the PostgreSQL database running in Docker. The application will load its configuration from `.env.development`. The `run-dev-app` target also ensures the dev database is up.

6.  **Develop:**
    Make your code changes. The `go run` command will need to be manually restarted to pick up changes. For auto-reloading during development, consider using a tool like `air` (setup for `air` is not covered by this `Makefile`).

7.  **Stop Development Database:**
    When you're done, stop the PostgreSQL container:
    ```bash
    make dev-db-down
    ```

## Running the Production-like Environment Locally

This workflow runs both the Go application and the PostgreSQL database in Docker containers, simulating a production setup using `docker-compose.yml`.

1.  **Configure Environment:**
    Copy `.env.example` to `.env.production` and customize it. Pay special attention to production secrets, OAuth settings, and `DB_SOURCE_PROD` if you plan to run host-based migrations. If the file is missing, `make` commands will guide you.

2.  **Start Services:**
    ```bash
    make prod-env-up
    ```
    This command builds the Go application Docker image (if not already built) and starts the `app` and `postgres_db` services using `docker-compose.yml` and `.env.production`.

3.  **Apply Database Migrations (if needed from host):**
    If you need to run migrations against this production-like database from your host machine:
    ```bash
    make migrate-up-prod
    ```
    This uses the `DB_SOURCE_PROD` variable from `.env.production`.

4.  **Access Application:**
    The application should be accessible at `http://localhost:${SERVER_PORT}` (where `SERVER_PORT` is the host port mapped in `docker-compose.yml`, which uses the `SERVER_PORT` from `.env.production`).

5.  **Stop Services:**
    When you're done:
    ```bash
    make prod-env-down
    ```

## Makefile Commands

The `Makefile` provides several commands to streamline development and management. Run `make help` to see all available commands and their descriptions.

### General Development
*   `make build`: Build the Go application binary.
*   `make run`: Run the Go application directly on the host (for quick tests, uses local env vars if set, does not manage DB).
*   `make run-dev-app`: Run the Go application locally, configured via `.env.development`, connecting to the Dockerized development database.
*   `make test`: Run Go tests.
*   `make coverage`: Run tests with coverage.
*   `make clean`: Clean build artifacts.
*   `make tidy`: Tidy `go.mod` and `go.sum`.
*   `make wire`: Generate Wire dependency injection code.

### Development Database & Migrations (using `.env.development`)
*   `make dev-db-up`: Start the development PostgreSQL container.
*   `make dev-db-down`: Stop the development PostgreSQL container.
*   `make logs-dev-db`: Show logs for the development PostgreSQL container.
*   `make seed-dev-db`: Populate the development database with sample data from `migrations/seed.sql`.
*   `make migrate-up`: Apply all database migrations to the development database.
*   `make migrate-down`: Revert the last database migration on the development database.
*   `make migrate-create NAME=<migration_name>`: Create new migration files (e.g., `make migrate-create NAME=add_users_table`).

### Production-like Environment & Migrations (using `.env.production`)
*   `make prod-env-up`: Start all services (app & db) for a production-like environment using Docker Compose.
*   `make prod-env-down`: Stop all services in the production-like environment.
*   `make logs-prod-env`: Show logs for all services in the production-like environment.
*   `make migrate-up-prod`: Apply all migrations to the production-like database (executed from host).
*   `make migrate-down-prod`: Revert the last migration on the production-like database (executed from host).

## Database Migrations

Database schema changes are managed using `golang-migrate`. Migration files are located in the `./migrations` directory.
*   Use `make migrate-create NAME=your_migration_name` to create new migration files.
*   Edit the generated `*.up.sql` and `*.down.sql` files.
*   Apply migrations using `make migrate-up` (for development against the dev DB) or `make migrate-up-prod` (for the production-like DB, run from host).
*   **Important Notes from original migration docs:**
    *   Always write both `up` and `down` migrations.
    *   Test your migrations thoroughly in a development environment.
    *   Once a migration has been applied to a shared environment (staging, production), **do not edit it**. If changes are needed, create a new migration to modify the schema.

## Testing

```bash
make test
```

## Linting

(Placeholder: Add linting commands if/when configured in the project)

---

This `README.md` provides a comprehensive guide to setting up and developing the Seattle Info Backend application.
Remember to keep your `.env.*` files secure and never commit them to the repository.
Refer to `make help` for a quick list of available commands.